package net_plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/eosspark/eos-go/chain/types"
	"github.com/eosspark/eos-go/common"
	"github.com/eosspark/eos-go/crypto"
	"github.com/eosspark/eos-go/crypto/ecc"
	"net"
	"sync"
	"time"
)

type possibleConnections byte

const (
	nonePossible      possibleConnections = 0
	producersPossible possibleConnections = 1 << 0
	specifiedPossible possibleConnections = 1 << 1
	anyPossible       possibleConnections = 1 << 2
)

type netPluginIMpl struct {
	ListenEndpoint string

	p2PAddress         string
	maxClientCount     uint32
	maxNodesPerHost    uint32
	numClients         uint32
	suppliedPeers      []string
	AllowedPeers       []ecc.PublicKey                  //< peer keys allowed to connect
	privateKeys        map[ecc.PublicKey]ecc.PrivateKey //< overlapping with producer keys, also authenticating non-producing nodes
	allowedConnections possibleConnections
	done               bool
	connectorCheck     time.Timer
	transactionCheck   time.Timer
	keepAliceTimer     time.Timer

	connectorPeriod            time.Duration
	txnExpPeriod               time.Duration
	respExpectedPeriod         time.Duration
	keepaliveInterval          time.Duration //32*time.Sencond
	peerAuthenticationInterval time.Duration //< Peer clock may be no more than 1 second skewed from our clock, including network latency.

	maxCleanupTimeMs    int
	networkVersionMatch bool
	chainID             common.ChainIdType
	nodeID              common.NodeIdType
	userAgentName       string

	useSocketReadWatermark bool

	LocalTxns []string //TODO NodeTransactionIndex

	peers      map[string]*Peer
	syncMaster *syncManager
	dispatcher *dispatchManager

	quitNetImpl chan struct{}

	//ChainPlugin *ChainPlugin

	//incomingTransactionAckSubscription chan //incomingTransactionAckSubscription  channel_type::handle

	loopWG sync.WaitGroup
}

func NewNetPluginIMpl() *netPluginIMpl {
	return &netPluginIMpl{
		maxClientCount:             0,
		maxNodesPerHost:            1,
		numClients:                 0,
		allowedConnections:         nonePossible,
		done:                       false,
		keepaliveInterval:          32 * time.Second,
		peerAuthenticationInterval: 1 * time.Second,
		maxCleanupTimeMs:           0,
		networkVersionMatch:        false,
		txnExpPeriod:               defTxnExpireWait,
		useSocketReadWatermark:     false,
		syncMaster:                 NewSyncManager(250),
		dispatcher:                 NewDispatchManager(),
		privateKeys:                make(map[ecc.PublicKey]ecc.PrivateKey),
		quitNetImpl:                make(chan struct{}),
	}
}

func (impl *netPluginIMpl) startListenLoop() {

	listen, err := net.Listen("tcp", impl.ListenEndpoint)
	if err != nil {
		fmt.Println(err)
		//errChan <- fmt.Errorf("peer init: listening %s: %s", p.Address, err)
	}
	fmt.Println("Listening on: ", impl.ListenEndpoint)

	defer func() {
		impl.loopWG.Done()
		listen.Close()
	}()

	//visitors := uint32(0)
	fromAddr := uint32(0) //TODO same host??
	for {
		con, err := listen.Accept()
		if err != nil {
			fmt.Printf("accepting connection on %s: %s\n", con.RemoteAddr().String(), err)
			//errChan <- fmt.Errorf("peer init: accepting connection on %s: %s", p.Address, err)
		}
		fmt.Println("Connected on:", con.RemoteAddr())

		paddr := con.RemoteAddr().String()
		_, ok := impl.peers[paddr]
		if ok {
			continue
		}

		if fromAddr < impl.maxNodesPerHost && (impl.maxClientCount == 0 || uint32(len(impl.peers)) < impl.maxClientCount) {
			newPeer := NewPeer(con, bufio.NewReader(con))
			impl.peers[paddr] = newPeer

			impl.loopWG.Add(1)
			go impl.peers[paddr].read(impl)

		} else {
			if fromAddr >= impl.maxNodesPerHost {
				fmt.Printf("Number of connections %d from %s exceeds limit\n", fromAddr+1, paddr)
				//fc_elog(logger, "Number of connections (${n}) from ${ra} exceeds limit", ("n", from_addr+1)("ra",paddr.to_string()))
			} else {
				fmt.Printf("Error max_client_count %d exceeded\n", impl.maxClientCount)
				//fc_elog(logger, "Error max_client_count ${m} exceeded",( "m", max_client_count) )
			}
			con.Close()
		}

		fmt.Println("peers: ", impl.peers)
	}

}

//func (impl *netPluginIMpl) connect(peer *Peer) {
//
//}

func (impl *netPluginIMpl) close(peer *Peer) {

	//c->peer_addr.empty( ) && c->socket->is_open()
	if impl.numClients == 0 { //numClients is for other peers connect us
		//fc_wlog( logger, "num_clients already at 0")
		fmt.Println("num_clients already at 0")
	} else {
		impl.numClients -= 1
	}

	delete(impl.peers, peer.peerAddr)
	peer.connection.Close()
}

func (impl *netPluginIMpl) countOpenSockets() int {
	return len(impl.peers)
}

func (impl *netPluginIMpl) sendAll(msg P2PMessage, verify func(p *Peer) bool) {
	for _, p := range impl.peers {
		if p.current() && verify(p) {
			p.write(msg)
		}
	}
}

func (impl *netPluginIMpl) AcceptedBlockHeader(block *types.BlockState) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", block->id))
	fmt.Printf("signed,id =%v", block.ID)
}

func (impl *netPluginIMpl) AcceptedBlock(block *types.BlockState) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", block.ID))
	fmt.Printf("signaled,id = %v\n", block.ID)
	impl.dispatcher.bcastBlock(impl, block.SignedBlock)

}

func (impl *netPluginIMpl) IrreversibleBlock(block *types.BlockState) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", block.ID))
}

func (impl *netPluginIMpl) AcceptedTransaction(md *types.TransactionMetadata) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", md.id))
	//      dispatcher.bcast_transaction(md.packed_trx)
	impl.dispatcher.bcastTransaction(impl, md.PackedTrx)
}

func (impl *netPluginIMpl) AppliedTransaction(txn *types.TransactionTrace) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", txn.id))
}

func (impl *netPluginIMpl) AcceptedConfirmation(head *types.HeaderConfirmation) {
	//fc_dlog(logger,"signaled, id = ${id}",("id", head.BlockId))
}

func (impl *netPluginIMpl) TransactionAck(results common.Tuple) {
	packedTrx := results[1].(types.PackedTransaction) //TODO  std::pair<fc::exception_ptr, packed_transaction_ptr>&
	if results[0] != nil {
		//fc_ilog(logger,"signaled NACK, trx-id = ${id} : ${why}",("id", id)("why", results.first->to_detail_string()));
		id := packedTrx.ID()
		impl.dispatcher.rejectedTransaction(&id)
		fmt.Println(id)
	} else {
		//fc_ilog(logger,"signaled ACK, trx-id = ${id}",("id", id));
		impl.dispatcher.bcastTransaction(impl, &packedTrx)
		//elog("transactoin: ${sig}",("sig",*results.second));
	}
}

func (impl *netPluginIMpl) startConnTimer() {
	defer impl.loopWG.Done()

	for {
		select {
		case <-time.After(impl.connectorPeriod):

			impl.connectionMonitor()
		}

	}
}
func (impl *netPluginIMpl) connectionMonitor() {
	//fmt.Println("connTimer: ", "connection monitor", impl.connectorPeriod)

}

func (impl *netPluginIMpl) startTxnTimer() {
	defer impl.loopWG.Done()

	for {
		select {
		case <-time.After(impl.txnExpPeriod):

			impl.expireTxns()
			//case <- err:
			//elog( "Error from transaction check monitor: ${m}",( "m", ec.message()));
			//start_txn_timer( )
		}
	}
}
func (impl *netPluginIMpl) expireTxns() {
	//fmt.Println("startTxnTimer():  ", "cleanup expired txns ", impl.txnExpPeriod)

}

// ticker Peer heartbeat
func (impl *netPluginIMpl) ticker() {
	defer impl.loopWG.Done()
	for {
		select {
		case <-time.After(impl.keepaliveInterval):
			//fmt.Println("ticker():  ", impl.keepaliveInterval)
			for _, peer := range impl.peers {
				peer.sendTimeTicker()
			}
		}
	}
}

// authenticatePeer determine if a peer is allowed to connect.
// Checks current connection mode and key authentication.
// return False if the peer should not connect, True otherwise.
func (impl *netPluginIMpl) authenticatePeer(msg *HandshakeMessage) bool {
	var allowedIt, privateIt, foundProducerKey bool

	if impl.allowedConnections == nonePossible {
		return false
	}
	if impl.allowedConnections == anyPossible {
		return true
	}
	if impl.allowedConnections&(producersPossible|specifiedPossible) != 0 {
		for _, pubkey := range impl.AllowedPeers {
			if pubkey == msg.Key {
				allowedIt = true
			}
		}
		_, privateIt = impl.privateKeys[msg.Key]

		//producer_plugin* pp = app().find_plugin<producer_plugin>();
		//if(pp != nullptr)
		//found_producer_key = pp->is_producer_key(msg.key);

		if allowedIt && privateIt && !foundProducerKey {
			//elog( "Peer ${peer} sent a handshake with an unauthorized key: ${key}.",
			//	("peer", msg.p2p_address)("key", msg.key));

			return false
		}
	}
	msgTime := msg.Time
	t := common.Now()
	if time.Duration(uint64((t-msgTime)))*time.Microsecond > impl.peerAuthenticationInterval {
		//elog( "Peer ${peer} sent a handshake with a timestamp skewed by more than ${time}.",
		//	("peer", msg.p2p_address)("time", "1 second")); // TODO Add to_variant for std::chrono::system_clock::duration
		fmt.Printf("Peer %s sent a handshake with a timestamp skewed by more than 1 second\n", msg.P2PAddress)
		return false
	}

	if msg.Signature.String() != crypto.NewSha256Nil().String() && msg.Token.String() != crypto.NewSha256Nil().String() {
		hash := crypto.Hash256(msg.Time)
		if !hash.Compare(msg.Token) {
			//elog( "Peer ${peer} sent a handshake with an invalid token.",
			//	("peer", msg.p2p_address))
			fmt.Printf("Peer %s sent a handshake with an invalid token.\n", msg.P2PAddress)
			return false
		}

		peerKey, err := msg.Signature.PublicKey(msg.Token.Bytes())
		if err != nil {
			//elog( "Peer ${peer} sent a handshake with an unrecoverable key.",
			//  ("peer", msg.p2p_address))
			fmt.Printf("Peer %s sent a handshake with an unrecoverable key.\n", msg.P2PAddress)
			return false
		}
		if (impl.allowedConnections&(producersPossible|specifiedPossible)) != 0 && peerKey.String() != msg.Key.String() {
			//elog( "Peer ${peer} sent a handshake with an unauthenticated key.",
			//	("peer", msg.p2p_address));
			fmt.Printf("Peer %s sent a handshake with an unauthenticated key.\n", msg.P2PAddress)
			return false
		}

	} else if impl.allowedConnections&(producersPossible|specifiedPossible) != 0 {
		//dlog( "Peer sent a handshake with blank signature and token, but this node accepts only authenticated connections.")
		fmt.Println("Peer sent a handshake with blank signature and token,but this node accepts only authenticate connections.")
		return false
	}

	return true

}

// getAuthenticationKey retrieve public key used to authenticate with peers.
// Finds a key to use for authentication.  If this node is a producer, use
// the front of the producer key map.  If the node is not a producer but has
// a configured private key, use it.  If the node is neither a producer nor has
// a private key, returns an empty key.
// On a node with multiple private keys configured, the key with the first
// numerically smaller byte will always be used.
func (impl *netPluginIMpl) getAuthenticationKey() *ecc.PublicKey {
	if len(impl.privateKeys) > 0 {
		for pubkey, _ := range impl.privateKeys { //TODO easier  ？？？
			return &pubkey
		}
		/*producer_plugin* pp = app().find_plugin<producer_plugin>(); //TODO EOSIO not used
		if(pp != nullptr && pp->get_state() == abstract_plugin::started)
		   return pp->first_producer_public_key();*/
		return &ecc.PublicKey{}
	}

	return &ecc.PublicKey{}
}

// signCompact returns a signature of the digest using the corresponding private key of the signer.
// If there are no configured private keys, returns an empty signature.
func (impl *netPluginIMpl) signCompact(signer *ecc.PublicKey, digest *crypto.Sha256) *ecc.Signature {
	privateKeyPtr, ok := impl.privateKeys[*signer]
	if ok {
		signature, err := privateKeyPtr.Sign(digest.Bytes())
		if err != nil {
			panic(err)
		}
		return &signature
	} else { //TODO producer_plugin
		//producerPlugin
		//
		//return pp.signCompact(signer,digest)

		//producer_plugin* pp = app().find_plugin<producer_plugin>();
		//if(pp != nullptr && pp->get_state() == abstract_plugin::started)
		//return pp->sign_compact(signer, digest);
	}
	return &ecc.Signature{}
}

func (impl *netPluginIMpl) handleChainSizeMsg(p *Peer, msg *ChainSizeMessage) {
	//peer_ilog(c, "received chain_size_message")
	fmt.Println("receives chain_size_message")
}

func (impl *netPluginIMpl) handleHandshakeMsg(p *Peer, msg *HandshakeMessage) {
	fmt.Println("receive a handshake message")
	if !isValid(msg) {
		fmt.Println("bad handshake message")
		goAwayMsg := &GoAwayMessage{
			Reason: fatalOther,
			NodeID: *crypto.NewSha256Nil(),
		}
		p.write(goAwayMsg)
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(p.peerAddr, ": receive a handshake message", string(data))

	//controller& cc = chain_plug->chain();
	//uint32_t lib_num = cc.last_irreversible_block_num( );
	//uint32_t peer_lib = msg.last_irreversible_block_num;

	//libNum := uint32(100) //TODO
	//peerLib := msg.LastIrreversibleBlockNum

	if msg.Generation == 1 {
		if crypto.Sha256(msg.NodeID).Compare(crypto.Sha256(p.nodeID)) {
			//elog( "Self connection detected. Closing connection")
			fmt.Println("Self connection detected. Closing connection")
			goAwayMsg := &GoAwayMessage{
				Reason: fatalOther,
				NodeID: *crypto.NewSha256Nil(),
			}
			p.write(goAwayMsg)
		}

		//TODO check for duplicate!!
		//if( c->peer_addr.empty() || c->last_handshake_recv.node_id == fc::sha256()) {
		//fc_dlog(logger, "checking for duplicate" )
		//}

		if msg.ChainID.String() != p2pChainIDString {
			//elog( "Peer on a different chain. Closing connection")
			fmt.Println("Peer on different chain. Closing connection")
			goAwayMsg := &GoAwayMessage{
				Reason: wrongChain,
				NodeID: *crypto.NewSha256Nil(),
			}
			p.write(goAwayMsg)
			return
		}

		p.protocolVersion = toProtocolVersion(msg.NetworkVersion)
		if p.protocolVersion != netVersion {
			if impl.networkVersionMatch {
				//elog("Peer network version does not match expected ${nv} but got ${mnv}",
				//	("nv", net_version)("mnv", c->protocol_version))
				fmt.Printf("Peer network version does not match expected %d but got %d", netVersion, p.protocolVersion)
				goAwayMsg := &GoAwayMessage{
					Reason: wrongVersion,
					NodeID: *crypto.NewSha256Nil(),
				}
				p.write(goAwayMsg)
				return
			} else {
				//ilog("Local network version: ${nv} Remote version: ${mnv}",
				//	("nv", net_version)("mnv", c->protocol_version))
				fmt.Printf("local network version: %d Remote version: %d", netVersion, p.protocolVersion)
			}
		}

		if p.nodeID.String() != msg.NodeID.String() {
			p.nodeID = msg.NodeID
		}

		//fmt.Println("authrnticatePeer")//TODO check for authenticatePeer!!!
		//if !np.authenticatePeer(msg){
		//	//elog("Peer not authenticated.  Closing connection.")
		//	fmt.Println("Peer not authenticated. Closing connection")
		//	goAwayMsg := &GoAwayMessage{
		//		Reason: authentication,
		//		NodeID: *crypto.NewSha256Nil(),
		//	}
		//	p.write(goAwayMsg)
		//	return
		//}

		//onFork := false//TODO check for onFork!!!
		////fc_dlog(logger, "lib_num = ${ln} peer_lib = ${pl}",("ln",lib_num)("pl",peer_lib));
		//if peerLib <= libNum && peerLib > 0 {
		//	//peerLibID := cc.getBlockIdForNum(peerLib)
		//	//onFork = msg.LastIrreversibleBlockID != peerLibID
		//	onFork = true
		//
		//	if onFork {
		//		//elog( "Peer chain is forked");
		//		fmt.Println("Peer chain is forked")
		//		goAwayMsg := &GoAwayMessage{
		//			Reason: forked,
		//			NodeID: *crypto.NewSha256Nil(),
		//		}
		//		p.write(goAwayMsg)
		//		return
		//	}
		//}

		if p.sentHandshakeCount == 0 {
			p.sendHandshake(impl)
		}
	}

	p.lastHandshakeRecv = msg
	impl.syncMaster.recvHandshake(impl, p, msg)

}

func (impl *netPluginIMpl) handleGoawayMsg(p *Peer, msg *GoAwayMessage) {
	rsn := ReasonToString[msg.Reason]
	fmt.Printf("receive go_away_message reason = %s\n", rsn)
	//ilog( "received a go away message from ${p}, reason = ${r}",
	//	("p", c->peer_name())("r",rsn));
	p.noRetry = msg.Reason
	if msg.Reason == duplicate {
		p.nodeID = common.NodeIdType(msg.NodeID)
	}
	//p.flushQueues()
	p.close()

}

// handleTimeMsg process time_message
// Calculate offset, delay and dispersion.  Note carefully the
// implied processing.  The first-order difference is done
// directly in 64-bit arithmetic, then the result is converted
// to floating double.  All further processing is in
// floating-double arithmetic with rounding done by the hardware.
// This is necessary in order to avoid overflow and preserve precision.

func (impl *netPluginIMpl) handleTimeMsg(p *Peer, msg *TimeMessage) {
	fmt.Println("receive time_message")
	/* We've already lost however many microseconds it took to dispatch
	 * the message, but it can't be helped.
	 */
	msg.Dst = common.Now()

	// If the transmit timestamp is zero, the peer is horribly broken.
	if msg.Xmt == 0 {
		return /* invalid timestamp */
	}
	if msg.Xmt == p.xmt {
		return /* duplicate packet */
	}

	p.xmt = msg.Xmt
	p.rec = msg.Rec
	p.dst = msg.Dst
	if msg.Org == 0 {
		p.sendTime(msg)
		return // We don't have enough data to perform the calculation yet.
	}

	//p.offset = float64((p.rec-p.org)+(msg.Xmt-p.dst)) / 2
	//fmt.Println(p.offset)

	//NsecPerUsec := float64(1000)
	//fmt.Printf("Clock offset is %v ns  %v us\n", p.offset, p.offset/NsecPerUsec)
	//if(logger.is_enabled(fc::log_level::all))
	//logger.log(FC_LOG_MESSAGE(all, "Clock offset is ${o}ns (${us}us)", ("o", c->offset)("us", c->offset/NsecPerUsec)));
	p.org = 0
	p.rec = 0

}
func (impl *netPluginIMpl) handleNoticeMsg(p *Peer, msg *NoticeMessage) {
	// peer tells us about one or more blocks or txns. When done syncing, forward on
	// notices of previously unknown blocks or txns,
	//peer_ilog(c, "received notice_message");

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(p.peerAddr, ": received notice_message", string(data))

	p.connecting = false
	req := RequestMessage{}
	sendReq := false

	if msg.KnownTrx.Mode != none {
		fmt.Printf("this is a %s notice with %d transactions \n",
			modeTostring[msg.KnownTrx.Mode], msg.KnownTrx.Pending)
	}

	switch msg.KnownTrx.Mode {
	case none:
	case lastIrrCatchUp:
		p.lastHandshakeRecv.HeadNum = msg.KnownTrx.Pending
		req.ReqTrx.Mode = none
	case catchUp:
		if msg.KnownTrx.Pending > 0 {
			//plan to get all except what we already know about
			req.ReqTrx.Mode = catchUp
			sendReq = true
			//knownSum := local_txns.size()
			//if( known_sum ) {
			//	for( const auto& t : local_txns.get<by_id>( ) ) {
			//	req.req_trx.ids.push_back( t.id )
			//	}
			//}
		}
	case normal:
		//np.Dispatcher.recvNotice(p,msg,false)
	}

	if msg.KnownBlocks.Mode != none {
		fmt.Printf("this is a %s notice with  %d blocks\n",
			modeTostring[msg.KnownBlocks.Mode], msg.KnownBlocks.Pending)
	}
	switch msg.KnownBlocks.Mode {
	case none:
		if msg.KnownTrx.Mode != normal {
			return
		}
	case lastIrrCatchUp, catchUp:
		impl.syncMaster.recvNotice(impl, p, msg)
	case normal:
		impl.dispatcher.recvNotice(impl, p, msg, false)
	default:
		//peer_elog(c, "bad notice_message : invalid known_blocks.mode ${m}",("m",static_cast<uint32_t>(msg.known_blocks.mode)));
		fmt.Printf("bad notice_message : invalid knwon_blocks.mode %d\n", msg.KnownBlocks.Mode)
	}
	//fc_dlog(logger, "send req = ${sr}", ("sr",send_req));

	if sendReq {
		p.write(&req)
	}

}

func (impl *netPluginIMpl) handleRequestMsg(p *Peer, msg *RequestMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(p.peerAddr, ": received request_message", string(data))

	switch msg.ReqBlocks.Mode {
	case catchUp:
		fmt.Println("received request_message:catch_up")
		//p.blkSendBranch()

	case normal:
		fmt.Println("receive request_message:normal")
		//c.blkSend(msg.ReqBlocks.IDs)

	default:

	}

	switch msg.ReqTrx.Mode {
	case catchUp:
		//c.txnSendPending(msg.ReqTrx.IDs)

	case normal:
		//c.txnSend(msg.ReqTrx.IDs)
	case none:
		if msg.ReqBlocks.Mode == none {
			//c.stopSend()
		}
	default:

	}
}

func (impl *netPluginIMpl) handleSyncRequestMsg(p *Peer, msg *SyncRequestMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(p.peerAddr, ": received sync_request_message", string(data))
	if msg.EndBlock == 0 {

		//c.peerRequested.reset()
		//c.flushQueues()
	} else {

		//c.peerRequested = syncState(msg.StartBlock,msg.EndBlock,msg.StartBlock-1)
		//c.enqueueSyncBlock()

	}

}
func (impl *netPluginIMpl) handleSignedBlock(p *Peer, msg *SignedBlockMessage) {
	//fmt.Println("receive signed_block message")
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(p.peerAddr, ": receive signed_block message", string(data))

	//cc := chain_plug->chain()
	blkID := msg.BlockID()
	blkNum := msg.BlockNumber()
	//fmt.Printf("canceling wait on %s\n",p.peerAddr)
	//p.CancalWait()

	//Try(func() {
	//	//if cc.FetchBlockByID(blkID) {
	//	//	np.SyncMaster.recvBlock(p,&blkID,blkNum)
	//	//	return
	//	//}
	//}).Catch(...){
	////// should this even be caught?
	////elog("Caught an unknown exception trying to recall blockID");
	//}

	//np.Dispatcher.recvBlock(p,blkID,blkNum)
	//age := common.Now()-common.TimePoint(msg.Timestamp)
	//peer_ilog(c, "received signed_block : #${n} block age in secs = ${age}",
	//	("n",blk_num)("age",age.to_seconds()));
	//fmt.Printf("received signed_block : %d  block age in secs = %s",blkNum,age)

	reason := GoAwayReason(fatalOther)

	//try {
	//	signed_block_ptr sbp = std::make_shared<signed_block>(msg);
	//	chain_plug->accept_block(sbp); //, sync_master->is_active(c));
	//	reason = no_reason;
	//} catch( const unlinkable_block_exception &ex) {
	//peer_elog(c, "bad signed_block : ${m}", ("m",ex.what()));
	//reason = unlinkable;
	//} catch( const block_validate_exception &ex) {
	//peer_elog(c, "bad signed_block : ${m}", ("m",ex.what()));
	//elog( "block_validate_exception accept block #${n} syncing from ${p}",("n",blk_num)("p",c->peer_name()));
	//reason = validation;
	//} catch( const assert_exception &ex) {
	//peer_elog(c, "bad signed_block : ${m}", ("m",ex.what()));
	//elog( "unable to accept block on assert exception ${n} from ${p}",("n",ex.to_string())("p",c->peer_name()));
	//} catch( const fc::exception &ex) {
	//peer_elog(c, "bad signed_block : ${m}", ("m",ex.what()));
	//elog( "accept_block threw a non-assert exception ${x} from ${p}",( "x",ex.to_string())("p",c->peer_name()));
	//reason = no_reason;
	//} catch( ...) {
	//peer_elog(c, "bad signed_block : unknown exception");
	//elog( "handle sync block caught something else from ${p}",("num",blk_num)("p",c->peer_name()));
	//}

	//chain_plug.accept_block(msg)
	reason = noReason

	//ubn :=NewupdateBlockNum(blkNum)
	if reason == noReason {
		//var id common.TransactionIdType
		//for _,recp := range msg.Transactions{
		//	if recp.Trx.TransactionID == common.TransactionIdType(*crypto.NewSha256Nil()){//TODO
		//		id = recp.Trx.TransactionID
		//	}else{
		//       id = recp.Trx.PackedTransaction.ID()
		//	}
		//	//auto ltx = local_txns.get<by_id>().find(id);
		//	//if( ltx != local_txns.end()) {
		//	//	local_txns.modify( ltx, ubn );
		//	//}
		//	//auto ctx = c->trx_state.get<by_id>().find(id);
		//	//if( ctx != c->trx_state.end()) {
		//	//	c->trx_state.modify( ctx, ubn );
		//	//}
		//}
		impl.syncMaster.recvBlock(impl, p, blkID, blkNum)
	} else {
		impl.syncMaster.rejectedBlock(impl, p, blkNum)
	}

}

func (impl *netPluginIMpl) handlePackTransaction(p *Peer, msg *PackedTransactionMessage) {
	fmt.Println("receive packed transaction")
	tid := msg.ID()
	fmt.Println(tid)

	//controller &cc = chain_plug->chain()
	// blk_id := msg.ID()
	// blk_num := msg.Bloc
	//fc_dlog(logger, "canceling wait on ${p}", ("p",c->peer_name()));
	//c->cancel_wait();
	//
	//try {
	//	if( cc.fetch_block_by_id(blk_id)) {
	//	sync_master->recv_block(c, blk_id, blk_num);
	//	return;
	//}
	//} catch( ...) {
	//// should this even be caught?
	//elog("Caught an unknown exception trying to recall blockID");
	//}
}

func (impl *netPluginIMpl) findConnection(host string) *Peer {
	for addr, p := range impl.peers {
		if addr == host {
			return p
		}
	}
	return &Peer{}
}

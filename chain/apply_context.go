package chain

import (
	"fmt"
	"github.com/eosspark/eos-go/chain/types"
	"github.com/eosspark/eos-go/common"
	arithmetic "github.com/eosspark/eos-go/common/arithmetic_types"
	"github.com/eosspark/eos-go/crypto"
	"github.com/eosspark/eos-go/crypto/ecc"
	"github.com/eosspark/eos-go/crypto/rlp"
	"github.com/eosspark/eos-go/database"
	"github.com/eosspark/eos-go/entity"
	. "github.com/eosspark/eos-go/exception"
	"github.com/eosspark/eos-go/log"
)

type ApplyContext struct {
	Control *Controller

	DB                 *database.LDataBase
	TrxContext         *TransactionContext
	Act                *types.Action
	Receiver           common.AccountName
	UsedAuthorizations []bool
	RecurseDepth       uint32
	Privileged         bool
	ContextFree        bool
	UsedContestFreeApi bool
	Trace              types.ActionTrace

	idx64     *Idx64
	idxDouble *IdxDouble
	// IDX128        GenericIndex
	// IDX256        GenericIndex
	// IDXLongDouble GenericIndex

	//GenericIndex
	KeyvalCache          *iteratorCache
	Notified             []common.AccountName
	InlineActions        []types.Action
	CfaInlineActions     []types.Action
	PendingConsoleOutput string
	AccountRamDeltas     types.FlatSet
}

func NewApplyContext(control *Controller, trxContext *TransactionContext, act *types.Action, recurseDepth uint32) *ApplyContext {

	applyContext := &ApplyContext{
		Control:            control,
		DB:                 (control.DB).(*database.LDataBase),
		TrxContext:         trxContext,
		Act:                act,
		Receiver:           act.Account,
		UsedAuthorizations: make([]bool, len(act.Authorization)), //to false
		RecurseDepth:       recurseDepth,

		Privileged:         false,
		ContextFree:        false,
		UsedContestFreeApi: false,

		KeyvalCache: NewIteratorCache(),
	}

	applyContext.idx64 = NewIdx64(applyContext)
	applyContext.idxDouble = NewIdxDouble(applyContext)

	return applyContext

}

type pairTableIterator struct {
	tableIDObject *entity.TableIdObject
	iterator      int
}

type iteratorCache struct {
	tableCache         map[common.IdType]*pairTableIterator
	endIteratorToTable []*entity.TableIdObject
	iteratorToObject   []interface{}
	objectToIterator   map[interface{}]int
}

func NewIteratorCache() *iteratorCache {
	i := iteratorCache{
		tableCache: make(map[common.IdType]*pairTableIterator),
		// endIteratorToTable: make([]*entity.TableIdObject, 8),
		// iteratorToObject:   make([]interface{}, 32),
		objectToIterator: make(map[interface{}]int),
	}
	return &i
}

func (i *iteratorCache) endIteratorToIndex(ei int) int    { return (-ei - 2) }
func (i *iteratorCache) IndexToEndIterator(index int) int { return -(index + 2) }
func (i *iteratorCache) cacheTable(tobj *entity.TableIdObject) int {
	if itr, ok := i.tableCache[tobj.ID]; ok {
		return itr.iterator
	}

	if len(i.endIteratorToTable) >= 8 {
		return 0 // an invalid iterator
	}

	ei := i.IndexToEndIterator(len(i.endIteratorToTable))
	i.endIteratorToTable = append(i.endIteratorToTable, tobj)

	pair := &pairTableIterator{
		tableIDObject: tobj,
		iterator:      ei,
	}
	i.tableCache[tobj.ID] = pair
	return ei
}
func (i *iteratorCache) getTable(id common.IdType) *entity.TableIdObject {
	if itr, ok := i.tableCache[id]; ok {
		return itr.tableIDObject
	}

	EosAssert(false, &TableNotInCache{}, "an invariant was broken, table should be in cache")
	return &entity.TableIdObject{}
}
func (i *iteratorCache) getEndIteratorByTableID(id common.IdType) int {
	if itr, ok := i.tableCache[id]; ok {
		return itr.iterator
	}
	EosAssert(false, &TableNotInCache{}, "an invariant was broken, table should be in cache")
	return -1
}
func (i *iteratorCache) findTablebyEndIterator(ei int) *entity.TableIdObject {
	EosAssert(ei < -1, &InvalidTableTterator{}, "not an end iterator")
	index := i.endIteratorToIndex(ei)
	if index >= len(i.endIteratorToTable) {
		return nil
	}
	return i.endIteratorToTable[index]
}
func (i *iteratorCache) get(iterator int) interface{} {
	EosAssert(iterator != -1, &InvalidTableTterator{}, "invalid iterator")
	EosAssert(iterator >= 0, &TableOperationNotPermitted{}, "dereference of end iterator")
	EosAssert(iterator < len(i.iteratorToObject), &InvalidTableTterator{}, "iterator out of range")
	obj := i.iteratorToObject[iterator]
	EosAssert(obj != nil, &TableOperationNotPermitted{}, "dereference of deleted object")
	return obj
}
func (i *iteratorCache) remove(iterator int) {
	EosAssert(iterator != -1, &InvalidTableTterator{}, "invalid iterator")
	EosAssert(iterator >= 0, &TableOperationNotPermitted{}, "dereference of end iterator")
	EosAssert(iterator < len(i.iteratorToObject), &InvalidTableTterator{}, "iterator out of range")
	obj := i.iteratorToObject[iterator]
	if obj == nil {
		return
	}
	i.iteratorToObject[iterator] = nil
	delete(i.objectToIterator, obj)
}

func (i *iteratorCache) add(obj interface{}) int {
	if itr, ok := i.objectToIterator[obj]; ok {
		return itr
	}

	if len(i.iteratorToObject) >= 32 {
		return -1
	}

	i.iteratorToObject = append(i.iteratorToObject, obj)
	i.objectToIterator[obj] = len(i.iteratorToObject) - 1
	return len(i.iteratorToObject) - 1
}

func (a *ApplyContext) printDebug(receiver common.AccountName, at *types.ActionTrace) {

	if len(at.Console) != 0 {
		prefix := fmt.Sprintf("\n[(%s,%s)->%s]", common.S(uint64(at.Act.Account)), common.S(uint64(at.Act.Name)), common.S(uint64(receiver)))
		fmt.Println(prefix, ": CONSOLE OUTPUT BEGIN =====================\n")
		fmt.Println(at.Console)
		fmt.Println(prefix, ": CONSOLE OUTPUT END   =====================")
	}

}

func (a *ApplyContext) execOne() (trace types.ActionTrace) {

	start := common.Now() //common.TimePoint.now()

	//cfg := a.Control.GetGlobalProperties().Configuration
	action := a.Control.GetAccount(a.Receiver)
	a.Privileged = action.Privileged
	native := a.Control.FindApplyHandler(a.Receiver, a.Act.Account, a.Act.Name)

	if native != nil {
		if a.TrxContext.CanSubjectivelyFail && a.Control.IsProducingBlock() {
			a.Control.CheckContractList(a.Receiver)
			a.Control.CheckActionList(a.Act.Account, a.Act.Name)
		}
		native(a)
	}

	if len(action.Code) > 0 &&
		!(a.Act.Account == common.DefaultConfig.SystemAccountName && a.Act.Name == common.ActionName(common.N("setcode"))) &&
		a.Receiver == common.DefaultConfig.SystemAccountName {

		if a.TrxContext.CanSubjectivelyFail && a.Control.IsProducingBlock() {
			a.Control.CheckContractList(a.Receiver)
			a.Control.CheckActionList(a.Act.Account, a.Act.Name)
		}
		//try
		a.Control.GetWasmInterface().Apply(&action.CodeVersion, action.Code, a)
		//}catch(const wasm_exit&){}
	}

	r := &types.ActionReceipt{}
	r.Receiver = a.Receiver
	r.ActDigest = crypto.Hash256(a.Act)
	r.GlobalSequence = a.nextGlobalSequence()
	r.RecvSequence = a.nextRecvSequence(a.Receiver)
	r.AuthSequence = make(map[common.AccountName]uint64)
	accountSequence := &entity.AccountSequenceObject{Name: a.Act.Account}
	//a.DB.Get("byName", accountSequence)
	r.CodeSequence = uint32(accountSequence.CodeSequence)
	r.AbiSequence = uint32(accountSequence.AbiSequence)

	for _, auth := range a.Act.Authorization {
		r.AuthSequence[auth.Actor] = a.nextAuthSequence(auth.Actor)
	}

	b := types.BaseActionTrace{Receipt: *r}
	t := &types.ActionTrace{}
	t.BaseActionTrace = b
	t.TrxId = a.TrxContext.ID
	t.BlockNum = a.Control.PendingBlockState().BlockNum
	t.BlockTime = common.NewBlockTimeStamp(a.Control.PendingBlockTime())
	t.ProducerBlockId = a.Control.PendingProducerBlockId()
	t.AccountRamDeltas = a.AccountRamDeltas
	//a.accountRamDeltas.clear()
	t.Act = *a.Act
	t.Console = a.PendingConsoleOutput

	a.TrxContext.Executed = append(a.TrxContext.Executed, *r)

	if a.Control.ContractsConsole() {
		a.printDebug(a.Receiver, t)
	}

	a.resetConsole()

	t.Elapsed = common.Now().Sub(start)

	return *t

}
func (a *ApplyContext) Exec() {

	a.Notified = append(a.Notified, a.Receiver)
	trace := a.execOne()
	for _, r := range a.Notified {
		a.Receiver = r
		trace.InlineTraces = append(trace.InlineTraces, a.execOne())
	}

	if len(a.CfaInlineActions) > 0 || len(a.InlineActions) > 0 {
		EosAssert(a.RecurseDepth < uint32(a.Control.GetGlobalProperties().Configuration.MaxInlineActionDepth),
			&TransactionException{},
			"inline action recursion depth reached")
	}

	for _, inlineAction := range a.CfaInlineActions {
		trace.InlineTraces = append(trace.InlineTraces, types.ActionTrace{})
		a.TrxContext.DispathAction(&trace.InlineTraces[len(trace.InlineTraces)-1], &inlineAction, inlineAction.Account, true, a.RecurseDepth+1)
	}

	for _, inlineAction := range a.InlineActions {
		trace.InlineTraces = append(trace.InlineTraces, types.ActionTrace{})
		a.TrxContext.DispathAction(&trace.InlineTraces[len(trace.InlineTraces)-1], &inlineAction, inlineAction.Account, true, a.RecurseDepth+1)
	}

}

//context action api
func (a *ApplyContext) GetActionData() []byte           { return a.Act.Data }
func (a *ApplyContext) GetReceiver() common.AccountName { return a.Receiver }
func (a *ApplyContext) GetCode() common.AccountName     { return a.Act.Account }
func (a *ApplyContext) GetAct() common.ActionName       { return a.Act.Name }

//func (a *ApplyContext) RequireAuthorizations(account common.AccountName) {}
func (a *ApplyContext) IsAccount(n int64) bool {
	account := entity.AccountObject{Name: common.AccountName(n)}
	return a.DB.Find("byName", account, &account) == nil
}

//context authorization api
func (a *ApplyContext) RequireAuthorization(account int64) {
	//return
	for k, v := range a.Act.Authorization {
		if v.Actor == common.AccountName(account) {
			a.UsedAuthorizations[k] = true
			return
		}
	}
	EosAssert(false, &MissingAuthException{}, "missing authority of %s", common.S(uint64(account)))
}
func (a *ApplyContext) HasAuthorization(account int64) bool {
	for _, v := range a.Act.Authorization {
		if v.Actor == common.AccountName(account) {
			return true
		}
	}
	return false
}
func (a *ApplyContext) RequireAuthorization2(account int64, permission int64) {
	for k, v := range a.Act.Authorization {
		if v.Actor == common.AccountName(account) && v.Permission == common.PermissionName(permission) {
			a.UsedAuthorizations[k] = true
			return
		}
	}
	EosAssert(false, &MissingAuthException{}, "missing authority of %s/%s", common.S(uint64(account)), common.S(uint64(permission)))
}

func (a *ApplyContext) HasReciptient(code int64) bool {
	for _, a := range a.Notified {
		if a == common.AccountName(code) {
			return true
		}
	}
	return false
}
func (a *ApplyContext) RequireRecipient(recipient int64) {
	if a.HasReciptient(recipient) {
		a.Notified = append(a.Notified, common.AccountName(recipient))
	}
}

//context transaction api
func (a *ApplyContext) ExecuteInline(action []byte) {

	act := types.Action{}
	rlp.DecodeBytes(action, &act)

	code := entity.AccountObject{Name: act.Account}
	err := a.DB.Find("byName", code, &code)
	EosAssert(err != nil, &ActionValidateException{},
		"inline action's code account %s does not exist", common.S(uint64(act.Account)))

	for _, auth := range act.Authorization {
		actor := entity.AccountObject{Name: auth.Actor}
		err := a.DB.Find("byName", actor, &actor)
		EosAssert(err != nil, &ActionValidateException{}, "inline action's authorizing actor %s does not exist", common.S(uint64(auth.Actor)))
		EosAssert(a.Control.GetAuthorizationManager().FindPermission(&auth) != nil, &ActionValidateException{},
			"inline action's authorizations include a non-existent permission:%s",
			auth) //todo permissionLevel print
	}

	if !a.Control.SkipAuthCheck() && !a.Privileged && act.Account != a.Receiver {

		f := a.TrxContext.CheckTime
		a.Control.GetAuthorizationManager().CheckAuthorization([]*types.Action{&act},
			[]*ecc.PublicKey{},
			[]*types.PermissionLevel{&types.PermissionLevel{a.Receiver, common.DefaultConfig.EosioCodeName}},
			common.Microseconds(a.Control.PendingBlockTime()-a.TrxContext.Published),
			&f,
			false)

	}

	a.InlineActions = append(a.InlineActions, act)

}
func (a *ApplyContext) ExecuteContextFreeInline(action []byte) {

	act := types.Action{}
	rlp.DecodeBytes(action, &act)
	code := entity.AccountObject{Name: act.Account}
	err := a.DB.Find("byName", code, &code)
	EosAssert(err != nil, &ActionValidateException{},
		"inline action's code account %s does not exist", common.S(uint64(act.Account)))

	EosAssert(len(act.Authorization) == 0, &ActionValidateException{},
		"context-free actions cannot have authorizations")

	a.CfaInlineActions = append(a.CfaInlineActions, act)
}

func (a *ApplyContext) ScheduleDeferredTransaction(sendId *arithmetic.Uint128, payer common.AccountName, trx []byte, replaceExisting bool) {
}
func (a *ApplyContext) CancelDeferredTransaction2(sendId *arithmetic.Uint128, sender common.AccountName) bool {
	return false
}

func (a *ApplyContext) CancelDeferredTransaction(sendId *arithmetic.Uint128) bool {
	return a.CancelDeferredTransaction2(sendId, a.Receiver)
}

func (a *ApplyContext) FindTable(code int64, scope int64, table int64) *entity.TableIdObject {
	tab := entity.TableIdObject{Code: common.AccountName(code),
		Scope: common.ScopeName(scope),
		Table: common.TableName(table),
	}

	err := a.DB.Find("byCodeScopeTable", tab, &tab)
	if err == nil {
		return &tab
	}
	return nil
}
func (a *ApplyContext) FindOrCreateTable(code int64, scope int64, table int64, payer int64) *entity.TableIdObject {

	tab := entity.TableIdObject{Code: common.AccountName(code),
		Scope: common.ScopeName(scope),
		Table: common.TableName(table),
		Payer: common.AccountName(payer)}
	err := a.DB.Find("byCodeScopeTable", tab, &tab)
	if err == nil {
		return &tab
	}

	a.UpdateDbUsage(common.AccountName(payer), int64(common.BillableSizeV("table_id_object")))
	a.DB.Insert(&tab)
	return &tab
}
func (a *ApplyContext) RemoveTable(tid entity.TableIdObject) {
	a.UpdateDbUsage(tid.Payer, -int64(common.BillableSizeV("table_id_object")))

	table := entity.TableIdObject{ID: tid.ID}
	a.DB.Remove(&table)
}

//context producer api
func (a *ApplyContext) SetProposedProducers(data []byte) int64 {

	producers := []types.ProducerKey{}
	rlp.DecodeBytes(data, &producers)

	EosAssert(len(producers) <= common.DefaultConfig.MaxProducers,
		&WasmExecutionError{},
		"Producer schedule exceeds the maximum producer count for this chain")

	uniqueProducers := make(map[types.ProducerKey]bool)
	for _, p := range producers {
		EosAssert(a.IsAccount(int64(p.AccountName)), &WasmExecutionError{}, "producer schedule includes a nonexisting account")
		EosAssert(p.BlockSigningKey.Valid(), &WasmExecutionError{}, "producer schedule includes an invalid key")
		if _, ok := uniqueProducers[p]; !ok {
			uniqueProducers[p] = true
		}
	}

	EosAssert(len(producers) == len(uniqueProducers), &WasmExecutionError{}, "duplicate producer name in producer schedule")
	return a.Control.SetProposedProducers(producers)

}

func (a *ApplyContext) GetActiveProducersInBytes() []byte {

	ap := a.Control.ActiveProducers()
	accounts := make([]types.ProducerKey, len(ap.Producers))
	for _, producer := range ap.Producers {
		accounts = append(accounts, producer)
	}

	bytes, _ := rlp.EncodeToBytes(accounts)
	return bytes

}

//context console api
func (a *ApplyContext) resetConsole() {
	a.PendingConsoleOutput = ""
}
func (a *ApplyContext) ContextAppend(str string) { a.PendingConsoleOutput += str }

//func (a *ApplyContext) GetActiveProducers() []common.AccountName { return }

func (a *ApplyContext) GetPackedTransaction() []byte {
	bytes, err := rlp.EncodeToBytes(a.TrxContext.Trx)
	if err != nil {
		return []byte{}
	}
	return bytes
}
func (a *ApplyContext) UpdateDbUsage(payer common.AccountName, delta int64) {
	if delta > 0 {
		if !a.Privileged || payer == a.Receiver {

			EosAssert(a.Control.IsRamBillingInNotifyAllowed() || a.Receiver == a.Act.Account,
				&SubjectiveBlockProductionException{},
				"Cannot charge RAM to other accounts during notify.")
			a.RequireAuthorization(int64(payer))
		}
	}

	a.AddRamUsage(payer, delta)

}
func (a *ApplyContext) GetAction(typ uint32, index int, bufferSize int) (int, []byte) {
	trx := a.TrxContext.Trx
	var a_ptr *types.Action
	if typ == 0 {
		if index >= len(trx.ContextFreeActions) {
			return -1, nil
		}
		a_ptr = trx.ContextFreeActions[index]
	} else if typ == 1 {
		if index >= len(trx.ContextFreeActions) {
			return -1, nil
		}
		a_ptr = trx.Actions[index]
	}

	EosAssert(a_ptr != nil, &ActionNotFoundException{}, "action is not found")

	s, _ := rlp.EncodeSize(a_ptr)
	if s <= bufferSize {
		bytes, _ := rlp.EncodeToBytes(a_ptr)
		return s, bytes
	}
	return s, nil

}
func (a *ApplyContext) GetContextFreeData(index int, bufferSize int) (int, []byte) {

	trx := a.TrxContext.Trx
	if index >= len(trx.ContextFreeData) {
		return -1, nil
	}
	s := len(trx.ContextFreeData[index])
	if bufferSize == 0 {
		return s, nil
	}
	copySize := common.Min(uint64(bufferSize), uint64(s))
	return int(copySize), trx.ContextFreeData[index][0:copySize]

}

//context database api
func (a *ApplyContext) DbStoreI64(scope int64, table int64, payer int64, id int64, buffer []byte) int {
	return a.dbStoreI64(int64(a.Receiver), scope, table, payer, id, buffer)
}
func (a *ApplyContext) dbStoreI64(code int64, scope int64, table int64, payer int64, id int64, buffer []byte) int {
	tab := a.FindOrCreateTable(code, scope, table, payer)
	tid := tab.ID

	obj := entity.KeyValueObject{
		TId:        tid,
		PrimaryKey: uint64(id),
		Value:      buffer,
		Payer:      common.AccountName(payer),
	}

	a.DB.Insert(&obj)
	a.DB.Modify(tab, func(t *entity.TableIdObject) {
		t.Count++
	})

	// int64_t billable_size = (int64_t)(buffer_size + config::billable_size_v<key_value_object>);
	billableSize := int64(len(buffer)) + int64(common.BillableSizeV("key_value_object"))
	a.UpdateDbUsage(common.AccountName(payer), billableSize)
	a.KeyvalCache.cacheTable(tab)
	return a.KeyvalCache.add(&obj)
}
func (a *ApplyContext) DbUpdateI64(iterator int, payer int64, buffer []byte) {

	obj := (a.KeyvalCache.get(iterator)).(*entity.KeyValueObject)
	objTable := a.KeyvalCache.getTable(obj.TId)
	EosAssert(objTable.Code == a.Receiver, &TableAccessViolation{}, "db access violation")

	overhead := common.BillableSizeV("key_value_object")
	oldSize := int64(len(obj.Value)) + int64(overhead)
	newSize := int64(len(buffer)) + int64(overhead)

	payerAccount := common.AccountName(payer)
	if payerAccount == common.AccountName(0) {
		payerAccount = obj.Payer
	}

	if obj.Payer != payerAccount {
		a.UpdateDbUsage(obj.Payer, -(oldSize))
		a.UpdateDbUsage(payerAccount, newSize)
	} else if oldSize != newSize {
		a.UpdateDbUsage(obj.Payer, newSize-oldSize)
	}

	a.DB.Modify(obj, func(obj *entity.KeyValueObject) {
		obj.Value = buffer
		obj.Payer = payerAccount
	})
}
func (a *ApplyContext) DbRemoveI64(iterator int) {
	obj := (a.KeyvalCache.get(iterator)).(*entity.KeyValueObject)
	objTable := a.KeyvalCache.getTable(obj.TId)

	EosAssert(objTable.Code == a.Receiver, &TableAccessViolation{}, "db access violation")

	// //   require_write_lock( table_obj.scope );
	billableSize := int64(len(obj.Value)) + int64(common.BillableSizeV("key_value_object"))
	a.UpdateDbUsage(obj.Payer, -billableSize)
	a.DB.Modify(objTable, func(t *entity.TableIdObject) {
		t.Count--
	})

	a.DB.Remove(obj)
	if objTable.Count == 0 {
		a.DB.Remove(objTable)
	}
	a.KeyvalCache.remove(iterator)
}
func (a *ApplyContext) DbGetI64(iterator int, buffer []byte, bufferSize int) int {

	obj := (a.KeyvalCache.get(iterator)).(*entity.KeyValueObject)
	s := len(obj.Value)

	if bufferSize == 0 {
		return s
	}

	copySize := int(common.Min(uint64(bufferSize), uint64(s)))
	copy(buffer[0:copySize], obj.Value[0:copySize])
	return copySize
}
func (a *ApplyContext) DbNextI64(iterator int, primary *uint64) int {

	if iterator < -1 {
		return -1
	}
	obj := (a.KeyvalCache.get(iterator)).(*entity.KeyValueObject)
	idx, _ := a.DB.GetIndex("byScopePrimary", obj)

	itr := idx.IteratorTo(obj)
	ok := itr.Next()

	objKeyval := entity.KeyValueObject{}
	if ok {
		itr.Data(&objKeyval)
	}

	if itr == idx.End() || objKeyval.TId != obj.TId {
		return a.KeyvalCache.getEndIteratorByTableID(obj.TId)
	}

	*primary = objKeyval.PrimaryKey
	return a.KeyvalCache.add(&objKeyval)
}

func (a *ApplyContext) DbPreviousI64(iterator int, primary *uint64) int {

	idx, err := a.DB.GetIndex("byScopePrimary", entity.KeyValueObject{})

	fmt.Println(err)

	if iterator < -1 {
		tab := a.KeyvalCache.findTablebyEndIterator(iterator)
		EosAssert(tab != nil, &InvalidTableTterator{}, "not a valid end iterator")

		itr, _ := idx.UpperBound(tab.ID)
		if idx.BeginIterator() == idx.End() || itr == idx.BeginIterator() {
			return -1
		}

		itr.Prev()
		objPrev := entity.KeyValueObject{}
		itr.Data(&objPrev)

		if objPrev.TId != tab.ID {
			return -1
		}

		*primary = objPrev.PrimaryKey
		return a.KeyvalCache.add(&objPrev)
	}

	obj := (a.KeyvalCache.get(iterator)).(*entity.KeyValueObject)
	itr := idx.IteratorTo(obj)
	itr.Prev()
	objPrev := entity.KeyValueObject{}
	itr.Data(&objPrev)

	if objPrev.TId != obj.TId {
		return -1
	}

	*primary = objPrev.PrimaryKey
	return a.KeyvalCache.add(&objPrev)
}
func (a *ApplyContext) DbFindI64(code int64, scope int64, table int64, id int64) int {

	tab := a.FindTable(code, scope, table)
	if tab == nil {
		return -1
	}

	tableEndItr := a.KeyvalCache.cacheTable(tab)

	obj := entity.KeyValueObject{
		TId:        tab.ID,
		PrimaryKey: uint64(id),
	}
	err := a.DB.Find("byScopePrimary", obj, &obj)

	if err != nil {
		return tableEndItr
	}
	return a.KeyvalCache.add(&obj)

}
func (a *ApplyContext) DbLowerboundI64(code int64, scope int64, table int64, id int64) int {

	tab := a.FindTable(code, scope, table)
	if tab == nil {
		return -1
	}

	tableEndItr := a.KeyvalCache.cacheTable(tab)

	obj := entity.KeyValueObject{TId: tab.ID, PrimaryKey: uint64(id)}
	idx, _ := a.DB.GetIndex("byScopePrimary", &obj)

	itr, _ := idx.LowerBound(&obj)
	if itr == idx.End() {
		return tableEndItr
	}

	objLowerbound := entity.KeyValueObject{}
	itr.Data(&objLowerbound)
	if objLowerbound.TId != tab.ID {
		return tableEndItr
	}

	return a.KeyvalCache.add(&objLowerbound)

}
func (a *ApplyContext) DbUpperboundI64(code int64, scope int64, table int64, id int64) int {

	tab := a.FindTable(code, scope, table)
	if tab == nil {
		return -1
	}

	tableEndItr := a.KeyvalCache.cacheTable(tab)

	obj := entity.KeyValueObject{TId: tab.ID, PrimaryKey: uint64(id)}
	idx, _ := a.DB.GetIndex("byScopePrimary", &obj)

	itr, _ := idx.UpperBound(&obj)
	if itr == idx.End() {
		return tableEndItr
	}

	objUpperbound := entity.KeyValueObject{}
	itr.Data(&objUpperbound)

	if objUpperbound.TId != tab.ID {
		return tableEndItr
	}

	return a.KeyvalCache.add(objUpperbound)

}
func (a *ApplyContext) DbEndI64(code int64, scope int64, table int64) int {

	tab := a.FindTable(code, scope, table)
	if tab == nil {
		return -1
	}

	return a.KeyvalCache.cacheTable(tab)
}

//index for sceondarykey
func (a *ApplyContext) Idx64Store(scope int64, table int64, payer int64, id int64, value *types.Uint64_t) int {
	return a.idx64.store(scope, table, payer, id, value)
}
func (a *ApplyContext) Idx64Remove(iterator int) {
	a.idx64.remove(iterator)
}
func (a *ApplyContext) Idx64Update(iterator int, payer int64, value *types.Uint64_t) {
	a.idx64.update(iterator, payer, value)
}
func (a *ApplyContext) Idx64FindSecondary(code int64, scope int64, table int64, secondary *types.Uint64_t, primary *uint64) int {
	//a.idx64.update(iterator, payer, value)
	return a.idx64.findSecondary(code, scope, table, secondary, primary)
}
func (a *ApplyContext) Idx64Lowerbound(code int64, scope int64, table int64, secondary *types.Uint64_t, primary *uint64) int {
	//a.idx64.update(iterator, payer, value)
	return a.idx64.lowerbound(code, scope, table, secondary, primary)
}
func (a *ApplyContext) Idx64Upperbound(code int64, scope int64, table int64, secondary *types.Uint64_t, primary *uint64) int {
	return a.idx64.upperbound(code, scope, table, secondary, primary)
}
func (a *ApplyContext) Idx64End(code int64, scope int64, table int64) int {
	return a.idx64.end(code, scope, table)
}
func (a *ApplyContext) Idx64Next(iterator int, primary *uint64) int {
	return a.idx64.next(iterator, primary)
}
func (a *ApplyContext) Idx64Previous(iterator int, primary *uint64) int {
	return a.idx64.previous(iterator, primary)
}
func (a *ApplyContext) Idx64FindPrimary(code int64, scope int64, table int64, secondary *types.Uint64_t, primary *uint64) int {
	//a.idx64.update(iterator, payer, value)
	return a.idx64.findPrimary(code, scope, table, secondary, primary)
}

func (a *ApplyContext) IdxDoubleStore(scope int64, table int64, payer int64, id int64, value *types.Float64_t) int {
	return a.idxDouble.store(scope, table, payer, id, value)
}
func (a *ApplyContext) IdxDoubleRemove(iterator int) {
	a.idxDouble.remove(iterator)
}
func (a *ApplyContext) IdxDoubleUpdate(iterator int, payer int64, value *types.Float64_t) {
	a.idxDouble.update(iterator, payer, value)
}
func (a *ApplyContext) IdxDoubleFindSecondary(code int64, scope int64, table int64, secondary *types.Float64_t, primary *uint64) int {
	return a.idxDouble.findSecondary(code, scope, table, secondary, primary)
}
func (a *ApplyContext) IdxDoubleLowerbound(code int64, scope int64, table int64, secondary *types.Float64_t, primary *uint64) int {
	return a.idxDouble.lowerbound(code, scope, table, secondary, primary)
}
func (a *ApplyContext) IdxDoubleUpperbound(code int64, scope int64, table int64, secondary *types.Float64_t, primary *uint64) int {
	return a.idxDouble.upperbound(code, scope, table, secondary, primary)
}
func (a *ApplyContext) IdxDoubleEnd(code int64, scope int64, table int64) int {
	return a.idxDouble.end(code, scope, table)
}
func (a *ApplyContext) IdxDoubleNext(iterator int, primary *uint64) int {
	return a.idxDouble.next(iterator, primary)
}
func (a *ApplyContext) IdxDoublePrevious(iterator int, primary *uint64) int {
	return a.idxDouble.previous(iterator, primary)
}
func (a *ApplyContext) IdxDoubleFindPrimary(code int64, scope int64, table int64, secondary *types.Float64_t, primary *uint64) int {
	return a.idxDouble.findPrimary(code, scope, table, secondary, primary)
}

func (a *ApplyContext) nextGlobalSequence() uint64 {

	p := a.Control.GetDynamicGlobalProperties()
	a.DB.Modify(p, func(dgp *entity.DynamicGlobalPropertyObject) {
		dgp.GlobalActionSequence++
	})
	return p.GlobalActionSequence
}

func (a *ApplyContext) nextRecvSequence(receiver common.AccountName) uint64 {

	rs := entity.AccountSequenceObject{Name: receiver}
	a.DB.Find("byName", rs, &rs)
	a.DB.Modify(&rs, func(mrs *entity.AccountSequenceObject) {
		mrs.RecvSequence++
	})
	return rs.RecvSequence
}

func (a *ApplyContext) nextAuthSequence(receiver common.AccountName) uint64 {

	rs := entity.AccountSequenceObject{Name: receiver}
	a.DB.Find("byName", rs, &rs)
	a.DB.Modify(&rs, func(mrs *entity.AccountSequenceObject) {
		mrs.AuthSequence++
	})
	return rs.AuthSequence
}

// void apply_context::add_ram_usage( account_name account, int64_t ram_delta ) {
//    trx_context.add_ram_usage( account, ram_delta );

//    auto p = _account_ram_deltas.emplace( account, ram_delta );
//    if( !p.second ) {
//       p.first->delta += ram_delta;
//    }
// }

func (a *ApplyContext) AddRamUsage(account common.AccountName, ramDelta int64) {

	a.TrxContext.AddRamUsage(account, ramDelta)

	// auto p = _account_ram_deltas.emplace( account, ram_delta );
	// if( !p.second ) {
	// 	p.first->delta += ram_delta;
	// }

	//a.AccountRamDeltas.Append(account, ramDelta)

}

func (a *ApplyContext) Expiration() int       { return int(a.TrxContext.Trx.Expiration) }
func (a *ApplyContext) TaposBlockNum() int    { return int(a.TrxContext.Trx.RefBlockNum) }
func (a *ApplyContext) TaposBlockPrefix() int { return int(a.TrxContext.Trx.RefBlockPrefix) }

//context system api
func (a *ApplyContext) CheckTime() {
	a.TrxContext.CheckTime()
}
func (a *ApplyContext) CurrentTime() int64 {
	return a.Control.PendingBlockTime().TimeSinceEpoch().Count()
}
func (a *ApplyContext) PublicationTime() int64 {
	return a.TrxContext.Published.TimeSinceEpoch().Count()
}

//context permission api
func (a *ApplyContext) GetPermissionLastUsed(account common.AccountName, permission common.PermissionName) int64 {

	am := a.Control.GetAuthorizationManager()
	return am.GetPermissionLastUsed(am.GetPermission(&types.PermissionLevel{Actor: account, Permission: permission})).TimeSinceEpoch().Count()
}
func (a *ApplyContext) GetAccountCreateTime(account common.AccountName) int64 {

	obj := entity.AccountObject{Name: account}
	err := a.DB.Find("byName", obj, &obj)
	EosAssert(err != nil, &ActionValidateException{}, "account '%s' does not exist", common.S(uint64(account)))

	return obj.CreationDate.ToTimePoint().TimeSinceEpoch().Count()
}

//context privileged api
func (a *ApplyContext) SetResourceLimits(
	account common.AccountName,
	ramBytes uint64,
	netWeight uint64,
	cpuWeigth uint64) {

}
func (a *ApplyContext) GetResourceLimits(
	account common.AccountName,
	ramBytes *uint64,
	netWeight *uint64,
	cpuWeigth *uint64) {
}
func (a *ApplyContext) SetBlockchainParametersPacked(parameters []byte) {

	cfg := common.Config{}
	rlp.DecodeBytes(parameters, &cfg)

	a.DB.Modify(a.Control.GetGlobalProperties(), func(gpo *entity.GlobalPropertyObject) {
		gpo.Configuration = cfg
	})

}

func (a *ApplyContext) GetBlockchainParametersPacked() []byte {
	gpo := a.Control.GetGlobalProperties()
	bytes, err := rlp.EncodeToBytes(gpo.Configuration)
	if err != nil {
		log.Error("EncodeToBytes is error detail:", err)
		return nil
	}
	return bytes
}
func (a *ApplyContext) IsPrivileged(n common.AccountName) bool {

	account := entity.AccountObject{Name: n}
	err := a.DB.Find("byName", account, &account)
	if err != nil {
		log.Error("IsPrivileged is error detail:", err)
		return false
	}
	return account.Privileged

}
func (a *ApplyContext) SetPrivileged(n common.AccountName, isPriv bool) {
	account := entity.AccountObject{Name: n}
	a.DB.Modify(&account, func(ao *entity.AccountObject) {
		ao.Privileged = isPriv
	})
}

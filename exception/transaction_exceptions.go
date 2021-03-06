package exception

type TransactionException struct{ logMessage }

func (TransactionException) ChainExceptions()       {}
func (TransactionException) TransactionExceptions() {}
func (TransactionException) Code() ExcTypes         { return 3040000 }
func (TransactionException) What() string           { return "Transaction exception" }

type TxDecompressionError struct{ logMessage }

func (TxDecompressionError) ChainExceptions()       {}
func (TxDecompressionError) TransactionExceptions() {}
func (TxDecompressionError) Code() ExcTypes         { return 3040001 }
func (TxDecompressionError) What() string           { return "Error decompressing transaction" }

type TxNoAction struct{ logMessage }

func (TxNoAction) ChainExceptions()       {}
func (TxNoAction) TransactionExceptions() {}
func (TxNoAction) Code() ExcTypes         { return 3040002 }
func (TxNoAction) What() string           { return "Transaction should have at least one normal action" }

type TxNoAuths struct{ logMessage }

func (TxNoAuths) ChainExceptions()       {}
func (TxNoAuths) TransactionExceptions() {}
func (TxNoAuths) Code() ExcTypes         { return 3040003 }
func (TxNoAuths) What() string {
	return "Transaction should have at least one required authority"
}

type CfaIrrelevantAuth struct{ logMessage }

func (CfaIrrelevantAuth) ChainExceptions()       {}
func (CfaIrrelevantAuth) TransactionExceptions() {}
func (CfaIrrelevantAuth) Code() ExcTypes         { return 3040004 }
func (CfaIrrelevantAuth) What() string           { return "Context-free action should have no required authority" }

type ExpiredTxException struct{ logMessage }

func (ExpiredTxException) ChainExceptions()       {}
func (ExpiredTxException) TransactionExceptions() {}
func (ExpiredTxException) Code() ExcTypes         { return 3040005 }
func (ExpiredTxException) What() string           { return "Expired Transaction" }

type TxExpTooFarException struct{ logMessage }

func (TxExpTooFarException) ChainExceptions()       {}
func (TxExpTooFarException) TransactionExceptions() {}
func (TxExpTooFarException) Code() ExcTypes         { return 3040006 }
func (TxExpTooFarException) What() string           { return "Transaction Expiration Too Far" }

type InvalidRefBlockException struct{ logMessage }

func (InvalidRefBlockException) ChainExceptions()       {}
func (InvalidRefBlockException) TransactionExceptions() {}
func (InvalidRefBlockException) Code() ExcTypes         { return 3040007 }
func (InvalidRefBlockException) What() string           { return "Invalid Reference Block" }

type TxDuplicate struct{ logMessage }

func (TxDuplicate) ChainExceptions()       {}
func (TxDuplicate) TransactionExceptions() {}
func (TxDuplicate) Code() ExcTypes         { return 3040008 }
func (TxDuplicate) What() string           { return "Duplicate transaction" }

type DeferredTxDuplicate struct{ logMessage }

func (DeferredTxDuplicate) ChainExceptions()       {}
func (DeferredTxDuplicate) TransactionExceptions() {}
func (DeferredTxDuplicate) Code() ExcTypes         { return 3040009 }
func (DeferredTxDuplicate) What() string           { return "Duplicate deferred transaction" }

type CfaInsideGeneratedTx struct{ logMessage }

func (CfaInsideGeneratedTx) ChainExceptions()       {}
func (CfaInsideGeneratedTx) TransactionExceptions() {}
func (CfaInsideGeneratedTx) Code() ExcTypes         { return 3040010 }
func (CfaInsideGeneratedTx) What() string {
	return "Context free action is not allowed inside generated transaction"
}

type TxNotFound struct{ logMessage }

func (TxNotFound) ChainExceptions()       {}
func (TxNotFound) TransactionExceptions() {}
func (TxNotFound) Code() ExcTypes         { return 3040011 }
func (TxNotFound) What() string           { return "The transaction can not be found" }

type TooManyTxAtOnce struct{ logMessage }

func (TooManyTxAtOnce) ChainExceptions()       {}
func (TooManyTxAtOnce) TransactionExceptions() {}
func (TooManyTxAtOnce) Code() ExcTypes         { return 3040012 }
func (TooManyTxAtOnce) What() string           { return "Pushing too many transactions at once" }

type TxTooBig struct{ logMessage }

func (TxTooBig) ChainExceptions()       {}
func (TxTooBig) TransactionExceptions() {}
func (TxTooBig) Code() ExcTypes         { return 3040013 }
func (TxTooBig) What() string           { return "Transaction is too big" }

type UnknownTransactionCompression struct{ logMessage }

func (UnknownTransactionCompression) ChainExceptions()       {}
func (UnknownTransactionCompression) TransactionExceptions() {}
func (UnknownTransactionCompression) Code() ExcTypes         { return 3040014 }
func (UnknownTransactionCompression) What() string           { return "Unknown transaction compression" }

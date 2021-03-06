package exception

type WalletException struct{ logMessage }

func (WalletException) ChainExceptions()  {}
func (WalletException) WalletExceptions() {}
func (WalletException) Code() ExcTypes    { return 3120000 }
func (WalletException) What() string {
	return "Invalid contract vm version"
}

type WalletExistException struct{ logMessage }

func (WalletExistException) ChainExceptions()  {}
func (WalletExistException) WalletExceptions() {}
func (WalletExistException) Code() ExcTypes    { return 3120001 }
func (WalletExistException) What() string      { return "Wallet already exists" }

type WalletNonexistentException struct{ logMessage }

func (WalletNonexistentException) ChainExceptions()  {}
func (WalletNonexistentException) WalletExceptions() {}
func (WalletNonexistentException) Code() ExcTypes    { return 3120002 }
func (WalletNonexistentException) What() string      { return "Nonexistent wallet" }

type WalletLockedException struct{ logMessage }

func (WalletLockedException) ChainExceptions()  {}
func (WalletLockedException) WalletExceptions() {}
func (WalletLockedException) Code() ExcTypes    { return 3120003 }
func (WalletLockedException) What() string      { return "Locked wallet" }

type WalletMissingPubKeyException struct{ logMessage }

func (WalletMissingPubKeyException) ChainExceptions()  {}
func (WalletMissingPubKeyException) WalletExceptions() {}
func (WalletMissingPubKeyException) Code() ExcTypes    { return 3120004 }
func (WalletMissingPubKeyException) What() string      { return "Missing public key" }

type WalletInvalidPasswordException struct{ logMessage }

func (WalletInvalidPasswordException) ChainExceptions()  {}
func (WalletInvalidPasswordException) WalletExceptions() {}
func (WalletInvalidPasswordException) Code() ExcTypes    { return 3120005 }
func (WalletInvalidPasswordException) What() string      { return "Invalid wallet password" }

type WalletNotAvailableException struct{ logMessage }

func (WalletNotAvailableException) ChainExceptions()  {}
func (WalletNotAvailableException) WalletExceptions() {}
func (WalletNotAvailableException) Code() ExcTypes    { return 3120006 }
func (WalletNotAvailableException) What() string      { return "No available wallet" }

type WalletUnlockedException struct{ logMessage }

func (WalletUnlockedException) ChainExceptions()  {}
func (WalletUnlockedException) WalletExceptions() {}
func (WalletUnlockedException) Code() ExcTypes    { return 3120007 }
func (WalletUnlockedException) What() string      { return "Already unlocked" }

type KeyExistException struct{ logMessage }

func (KeyExistException) ChainExceptions()  {}
func (KeyExistException) WalletExceptions() {}
func (KeyExistException) Code() ExcTypes    { return 3120008 }
func (KeyExistException) What() string      { return "Key already exists" }

type KeyNonexistentException struct{ logMessage }

func (KeyNonexistentException) ChainExceptions()  {}
func (KeyNonexistentException) WalletExceptions() {}
func (KeyNonexistentException) Code() ExcTypes    { return 3120009 }
func (KeyNonexistentException) What() string      { return "Nonexistent key" }

type UnsupportedKeyTypeException struct{ logMessage }

func (UnsupportedKeyTypeException) ChainExceptions()  {}
func (UnsupportedKeyTypeException) WalletExceptions() {}
func (UnsupportedKeyTypeException) Code() ExcTypes    { return 3120010 }
func (UnsupportedKeyTypeException) What() string      { return "Unsupported key type" }

type InvalidLockTimeoutException struct{ logMessage }

func (InvalidLockTimeoutException) ChainExceptions()  {}
func (InvalidLockTimeoutException) WalletExceptions() {}
func (InvalidLockTimeoutException) Code() ExcTypes    { return 3120011 }
func (InvalidLockTimeoutException) What() string {
	return "Wallet lock timeout is invalid"
}

type SecureEnclaveException struct{ logMessage }

func (SecureEnclaveException) ChainExceptions()  {}
func (SecureEnclaveException) WalletExceptions() {}
func (SecureEnclaveException) Code() ExcTypes    { return 3120012 }
func (SecureEnclaveException) What() string {
	return "Secure Enclave Exception"
}

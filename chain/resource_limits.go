package chain

import (
	"github.com/eosspark/eos-go/chain/types"
	"github.com/eosspark/eos-go/common"
	"github.com/eosspark/eos-go/database"
	"github.com/eosspark/eos-go/entity"
	"github.com/eosspark/eos-go/common/arithmetic_types"
	. "github.com/eosspark/eos-go/exception"
	"fmt"
	"math"
)

var IsActiveRc bool

var rcInstance *ResourceLimitsManager

type ResourceLimitsManager struct {
	db database.DataBase `json:"db"`
}

func GetResourceLimitsManager() *ResourceLimitsManager {
	if !IsActiveRc {
		rcInstance = newResourceLimitsManager()
	}
	return rcInstance
}

func newResourceLimitsManager() *ResourceLimitsManager {
	IsActiveRc = true
	//control := GetControllerInstance()
	//db := control.DataBase()
	db, _ := database.NewDataBase(common.DefaultConfig.DefaultStateDirName)
	return &ResourceLimitsManager{db: db}
}

func (r *ResourceLimitsManager) InitializeDatabase() {
	config := entity.NewResourceLimitsConfigObject()
	r.db.Insert(&config)
	state := entity.DefaultResourceLimitsStateObject
	state.VirtualCpuLimit = config.CpuLimitParameters.Max
	state.VirtualNetLimit = config.NetLimitParameters.Max
	r.db.Insert(&state)
}

func (r *ResourceLimitsManager) InitializeAccount(account common.AccountName) {
	bl := entity.ResourceLimitsObject{}
	bl.Owner = account
	r.db.Insert(&bl)

	bu := entity.ResourceUsageObject{}
	bu.Owner = account
	r.db.Insert(&bu)
}

func (r *ResourceLimitsManager) SetBlockParameters(cpuLimitParameters types.ElasticLimitParameters, netLimitParameters types.ElasticLimitParameters) {
	cpuLimitParameters.Validate()
	netLimitParameters.Validate()
	config := entity.DefaultResourceLimitsConfigObject
	r.db.Find("id", config, &config)
	r.db.Modify(&config, func(c *entity.ResourceLimitsConfigObject) {
		c.CpuLimitParameters = cpuLimitParameters
		c.NetLimitParameters = netLimitParameters
	})
}

func (r *ResourceLimitsManager) UpdateAccountUsage(account []common.AccountName, timeSlot uint32) { //待定
	config := entity.DefaultResourceLimitsConfigObject
	r.db.Find("id", config, &config)
	usage := entity.ResourceUsageObject{}
	for _, a := range account {
		usage.Owner = a
		r.db.Find("byOwner", usage, &usage)
		r.db.Modify(&usage, func(bu *entity.ResourceUsageObject) {
			bu.NetUsage.Add(0, timeSlot, config.AccountNetUsageAverageWindow)
			bu.CpuUsage.Add(0, timeSlot, config.AccountCpuUsageAverageWindow)
		})
	}
	r.db.Find("byOwner", usage, &usage)
	fmt.Println(usage)
}

func (r *ResourceLimitsManager) AddTransactionUsage(account []common.AccountName, cpuUsage uint64, netUsage uint64, timeSlot uint32) {
	state := entity.DefaultResourceLimitsStateObject
	r.db.Find("id", state, &state)
	config := entity.DefaultResourceLimitsConfigObject
	r.db.Find("id", config, &config)
	for _, a := range account {
		usage := entity.ResourceUsageObject{}
		usage.Owner = a
		r.db.Find("byOwner", usage, &usage)
		var unUsed, netWeight, cpuWeight int64
		r.GetAccountLimits(a, &unUsed, &netWeight, &cpuWeight)
		r.db.Modify(&usage, func(bu *entity.ResourceUsageObject) {
			bu.CpuUsage.Add(netUsage, timeSlot, config.AccountNetUsageAverageWindow)
			bu.NetUsage.Add(cpuUsage, timeSlot, config.AccountCpuUsageAverageWindow)
		})

		if cpuWeight >= 0 && state.TotalCpuWeight > 0 {
			windowSize := uint64(config.AccountCpuUsageAverageWindow)
			virtualNetworkCapacityInWindow := arithmeticTypes.MulUint64(state.VirtualCpuLimit, windowSize)
			cpuUsedInWindow := arithmeticTypes.MulUint64(usage.CpuUsage.ValueEx, windowSize)
			cpuUsedInWindow, _ = cpuUsedInWindow.Div(arithmeticTypes.Uint128{0, uint64(common.DefaultConfig.RateLimitingPrecision)})
			userWeight := arithmeticTypes.Uint128{0, uint64(cpuWeight)}
			allUserWeight :=  arithmeticTypes.Uint128{0, state.TotalCpuWeight}

			maxUserUseInWindow := virtualNetworkCapacityInWindow.Mul(userWeight)
			maxUserUseInWindow, _ = maxUserUseInWindow.Div(allUserWeight)
			EosAssert(cpuUsedInWindow.Compare(maxUserUseInWindow) < 1, &TxCpuUsageExceed{},
			"authorizing account %s has insufficient cpu resources for this transaction,\n cpu_used_in_window: %s,\n max_user_use_in_window: %s",
			a, cpuUsedInWindow, maxUserUseInWindow)
		}

		if netWeight >= 0 && state.TotalNetWeight > 0 {
			windowSize := uint64(config.AccountNetUsageAverageWindow)
			virtualNetworkCapacityInWindow := arithmeticTypes.MulUint64(state.VirtualNetLimit, windowSize)
			netUsedInWindow := arithmeticTypes.MulUint64(usage.NetUsage.ValueEx, windowSize)
			netUsedInWindow, _ = netUsedInWindow.Div(arithmeticTypes.Uint128{0, uint64(common.DefaultConfig.RateLimitingPrecision)})
			userWeight := arithmeticTypes.Uint128{0, uint64(cpuWeight)}
			allUserWeight :=  arithmeticTypes.Uint128{0, state.TotalCpuWeight}

			maxUserUseInWindow := virtualNetworkCapacityInWindow.Mul(userWeight)
			maxUserUseInWindow, _ = maxUserUseInWindow.Div(allUserWeight)
			EosAssert(netUsedInWindow.Compare(maxUserUseInWindow) < 1, &TxCpuUsageExceed{},
				"authorizing account %s has insufficient cpu resources for this transaction,\n net_used_in_window: %s,\n max_user_use_in_window: %s",
				a, netUsedInWindow, maxUserUseInWindow)
		}
	}

	r.db.Modify(&state, func(rls *entity.ResourceLimitsStateObject) {
		rls.PendingCpuUsage += cpuUsage
		rls.PendingNetUsage += netUsage
	})
}

func (r *ResourceLimitsManager) AddPendingRamUsage(account common.AccountName, ramDelta int64) {
	if ramDelta == 0 {
		return
	}

	usage := entity.ResourceUsageObject{}
	usage.Owner = account
	r.db.Find("byOwner", usage, &usage)

	EosAssert(ramDelta <= 0 || math.MaxUint64-usage.RamUsage >= uint64(ramDelta), &TransactionException{},
		"Ram usage delta would overflow UINT64_MAX")
	EosAssert(ramDelta >= 0 && usage.RamUsage >= uint64(-ramDelta), &TransactionException{},
		"Ram usage delta would underflow UINT64_MAX")

	r.db.Modify(&usage, func(u *entity.ResourceUsageObject) {
		u.RamUsage += uint64(ramDelta)
	})
}

func (r *ResourceLimitsManager) VerifyAccountRamUsage(account common.AccountName) {
	var ramBytes, netWeight, cpuWeight int64
	r.GetAccountLimits(account, &ramBytes, &netWeight, &cpuWeight)
	usage := entity.ResourceUsageObject{}
	usage.Owner = account
	r.db.Find("byOwner", usage, &usage)

	if ramBytes >= 0 {
		EosAssert(usage.RamUsage <= uint64(ramBytes), &RamUsageExceeded{},
		"account %s has insufficient ram; needs %d bytes has %d bytes", account, usage.RamUsage, ramBytes)
	}
}

func (r *ResourceLimitsManager) GetAccountRamUsage(account common.AccountName) int64 {
	usage := entity.ResourceUsageObject{}
	usage.Owner = account
	r.db.Find("byOwner", usage, &usage)
	return int64(usage.RamUsage)
}

func (r *ResourceLimitsManager) SetAccountLimits(account common.AccountName, ramBytes int64, netWeight int64, cpuWeight int64) bool { //for test

	findOrCreatePendingLimits := func() entity.ResourceLimitsObject{
		pendingLimits := entity.ResourceLimitsObject{}
		pendingLimits.Owner = account
		pendingLimits.Pending = true
		err := r.db.Find("byOwner", pendingLimits, &pendingLimits)
		if err != nil{
			limits := entity.ResourceLimitsObject{}
			limits.Owner = account
			limits.Pending = false
			r.db.Find("byOwner", limits, &limits)
			pendingLimits.Owner = limits.Owner
			pendingLimits.RamBytes = limits.RamBytes
			pendingLimits.NetWeight = limits.NetWeight
			pendingLimits.CpuWeight = limits.CpuWeight
			pendingLimits.Pending = true
			r.db.Insert(&pendingLimits)
			return pendingLimits
		} else {
			return pendingLimits
		}
	}

	limits := findOrCreatePendingLimits()
	fmt.Println(limits)
	decreasedLimit := false
	if ramBytes >= 0 {
		decreasedLimit = limits.RamBytes < 0 || ramBytes < limits.RamBytes
	}

	r.db.Modify(&limits, func(pendingLimits *entity.ResourceLimitsObject) {
		pendingLimits.RamBytes = ramBytes
		pendingLimits.NetWeight = netWeight
		pendingLimits.CpuWeight = cpuWeight
	})
	return decreasedLimit
}

func (r *ResourceLimitsManager) GetAccountLimits(account common.AccountName, ramBytes *int64, netWeight *int64, cpuWeight *int64) {
	pendingBuo := entity.ResourceLimitsObject{}
	pendingBuo.Owner = account
	pendingBuo.Pending = true
	err := r.db.Find("byOwner", pendingBuo, &pendingBuo)
	if err == nil {
		*ramBytes = pendingBuo.RamBytes
		*netWeight = pendingBuo.NetWeight
		*cpuWeight = pendingBuo.CpuWeight
	} else {
		buo := entity.ResourceLimitsObject{}
		buo.Owner = account
		buo.Pending = false
		r.db.Find("byOwner", buo, &buo)
		fmt.Println(buo)
		*ramBytes = buo.RamBytes
		*netWeight = buo.NetWeight
		*cpuWeight = buo.CpuWeight
	}
}

func (r *ResourceLimitsManager) ProcessAccountLimitUpdates() {

	updateStateAndValue := func(total *uint64, value *int64, pendingValue int64, debugWhich string) {
		if *value > 0 {
			EosAssert(*total >= uint64(*value), &RateLimitingStateInconsistent{}, "underflow when reverting old value to %s", debugWhich)
			*total -= uint64(*value)
		}

		if pendingValue > 0 {
			EosAssert(math.MaxUint16 - *total >= uint64(pendingValue), &RateLimitingStateInconsistent{}, "overflow when applying new value to %s", debugWhich )
			*total += uint64(pendingValue)
		}

		*value = pendingValue
	}

	state := entity.ResourceLimitsStateObject{}
	r.db.Find("id", state, &state)
	r.db.Modify(&state, func(rso entity.ResourceLimitsStateObject) {
		//for _, itr := range pendingRlo {
		//	rlo := ResourceLimitsObject{}
		//	r.db.Find("Rlo", RloIndex{ResourceLimits, itr.Owner, false}, &rlo)
		//	r.db.Modify(&rlo, func(rlo entity.ResourceLimitsObject) {
		//		updateStateAndValue(&rso.TotalRamBytes, &rlo.RamBytes, itr.RamBytes, "ram_bytes")
		//		updateStateAndValue(&rso.TotalCpuWeight, &rlo.CpuWeight, itr.CpuWeight, "cpu_weight")
		//		updateStateAndValue(&rso.TotalNetWeight, &rlo.NetWeight, itr.NetWeight, "net_weight")
		//	})
		//}
	})
}

func (r *ResourceLimitsManager) ProcessBlockUsage(blockNum uint32) {
	//config := entity.ResourceLimitsConfigObject{}
	//r.db.Find("byId", &config)
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//r.db.Modify(&state, func(data interface{}) error {
	//
	//	state.AverageBlockCpuUsage.Add(state.PendingCpuUsage, blockNum, config.CpuLimitParameters.Periods)
	//	state.UpdateVirtualCpuLimit(config)
	//	state.PendingCpuUsage = 0
	//
	//	state.AverageBlockNetUsage.Add(state.PendingNetUsage, blockNum, config.NetLimitParameters.Periods)
	//	state.UpdateVirtualNetLimit(config)
	//	state.PendingNetUsage = 0
	//
	//	return nil
	//})
}

func (r *ResourceLimitsManager) GetVirtualBlockCpuLimit() uint64 {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//return state.VirtualCpuLimit
	return 0
}

func (r *ResourceLimitsManager) GetVirtualBlockNetLimit() uint64 {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//return state.VirtualNetLimit
	return 0
}

func (r *ResourceLimitsManager) GetBlockCpuLimit() uint64 {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//config := entity.ResourceLimitsConfigObject{}
	//r.db.Find("byId", &config)
	//return config.CpuLimitParameters.Max - state.PendingCpuUsage
	return 0
}

func (r *ResourceLimitsManager) GetBlockNetLimit() uint64 {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//config := entity.ResourceLimitsConfigObject{}
	//r.db.Find("byId", &config)
	//return config.NetLimitParameters.Max - state.PendingNetUsage
	return 0
}

func (r *ResourceLimitsManager) GetAccountCpuLimit(name common.AccountName, elastic bool) int64 {
	arl := r.GetAccountCpuLimitEx(name, elastic)
	return arl.Available
}

func (r *ResourceLimitsManager) GetAccountCpuLimitEx(name common.AccountName, elastic bool) AccountResourceLimit {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//config := entity.ResourceLimitsConfigObject{}
	//r.db.Find("byId", &config)
	//ruo := entity.ResourceUsageObject{}
	//r.db.Find("byOwner", &ruo)
	//
	//var cpuWeight, x, y int64
	//r.GetAccountLimits(name, &x, &y, &cpuWeight)
	//
	//if cpuWeight < 0 || state.TotalCpuWeight == 0 {
	//	return AccountResourceLimit{-1, -1, -1}
	//}

	arl := AccountResourceLimit{}
	//windowSize := new(big.Int).SetUint64(uint64(config.AccountCpuUsageAverageWindow))
	//virtualCpuCapacityInWindow := new(big.Int)
	//if elastic {
	//	virtualCpuCapacityInWindow = new(big.Int).Mul(new(big.Int).SetUint64(state.VirtualCpuLimit), windowSize)
	//} else {
	//	virtualCpuCapacityInWindow = new(big.Int).Mul(new(big.Int).SetUint64(config.CpuLimitParameters.Max), windowSize)
	//}
	//userWeight := new(big.Int).SetUint64(uint64(cpuWeight))
	//allUserWeight := new(big.Int).SetUint64(state.TotalCpuWeight)
	//
	//maxUserUseInWindow := new(big.Int).Div(new(big.Int).Mul(virtualCpuCapacityInWindow, userWeight), allUserWeight)
	//cpuUsedInWindow := IntegerDivideCeil(
	//	new(big.Int).Mul(new(big.Int).SetUint64(ruo.CpuUsage.ValueEx), windowSize),
	//	new(big.Int).SetUint64(uint64(common.DefaultConfig.RateLimitingPrecision)))
	//
	//if maxUserUseInWindow.Cmp(cpuUsedInWindow) != 1 {
	//	arl.Available = 0
	//} else {
	//	arl.Available = DowngradeCast(new(big.Int).Sub(maxUserUseInWindow, cpuUsedInWindow))
	//}
	//
	//arl.Used = DowngradeCast(cpuUsedInWindow)
	//arl.Max = DowngradeCast(maxUserUseInWindow)
	return arl
}

func (r *ResourceLimitsManager) GetAccountNetLimit(name common.AccountName, elastic bool) int64 {
	arl := r.GetAccountNetLimitEx(name, elastic)
	return arl.Available
}

func (r *ResourceLimitsManager) GetAccountNetLimitEx(name common.AccountName, elastic bool) AccountResourceLimit {
	//state := entity.ResourceLimitsStateObject{}
	//r.db.Find("byId", &state)
	//config := entity.ResourceLimitsConfigObject{}
	//r.db.Find("byId", &config)
	//ruo := entity.ResourceUsageObject{}
	//r.db.Find("byOwner", &ruo)
	//
	//var netWeight, x, y int64
	//r.GetAccountLimits(name, &x, &y, &netWeight)
	//
	//if netWeight < 0 || state.TotalNetWeight == 0 {
	//	return AccountResourceLimit{-1, -1, -1}
	//}

	arl := AccountResourceLimit{}
	//windowSize := new(big.Int).SetUint64(uint64(config.AccountNetUsageAverageWindow))
	//virtualNetCapacityInWindow := new(big.Int)
	//if elastic {
	//	virtualNetCapacityInWindow = new(big.Int).Mul(new(big.Int).SetUint64(state.VirtualNetLimit), windowSize)
	//} else {
	//	virtualNetCapacityInWindow = new(big.Int).Mul(new(big.Int).SetUint64(config.NetLimitParameters.Max), windowSize)
	//}
	//userWeight := new(big.Int).SetUint64(uint64(netWeight))
	//allUserWeight := new(big.Int).SetUint64(state.TotalNetWeight)
	//
	//maxUserUseInWindow := new(big.Int).Div(new(big.Int).Mul(virtualNetCapacityInWindow, userWeight), allUserWeight)
	//netUsedInWindow := IntegerDivideCeil(
	//	new(big.Int).Mul(new(big.Int).SetUint64(ruo.NetUsage.ValueEx), windowSize),
	//	new(big.Int).SetUint64(uint64(common.DefaultConfig.RateLimitingPrecision)))
	//if maxUserUseInWindow.Cmp(netUsedInWindow) != 1 {
	//	arl.Available = 0
	//} else {
	//	arl.Available = DowngradeCast(new(big.Int).Sub(maxUserUseInWindow, netUsedInWindow))
	//}
	//
	//arl.Used = DowngradeCast(netUsedInWindow)
	//arl.Max = DowngradeCast(maxUserUseInWindow)
	return arl
}

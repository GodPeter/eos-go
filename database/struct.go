package database

import (
	"fmt"
	"reflect"
)

/////////////////////////////////////////////////////// test struct ///////////////////////////////////////////
type Carnivore struct {
	Lion  int `multiIndex:"orderedNonUnique,greater"`
	Tiger int `multiIndex:"orderedNonUnique,greater"`
}

type DbHouse struct {
	Id        uint64 `multiIndex:"id,increment"`
	Area      uint64 `multiIndex:"orderedUnique,greater"`
	Name      string
	Carnivore Carnivore `multiIndex:"inline"`
}

type IdType int64
type Name uint64
type AccountName uint64
type PermissionName uint64
type ActionName uint64
type TableName uint64
type ScopeName uint64

type DbTableIdObject struct {
	ID    IdType      `multiIndex:"id,increment,byScope"`
	Code  AccountName `multiIndex:"orderedNonUnique,less"`
	Scope ScopeName   `multiIndex:"byTable,orderedNonUnique,greater:byScope,orderedNonUnique,less"`
	Table TableName   `multiIndex:"byTable,orderedNonUnique,greater"`
	Payer AccountName `multiIndex:"byScope,orderedNonUnique"`
	Count uint32
}

type DbResourceLimitsObject struct {
	ID        IdType      `multiIndex:"id,increment"`
	Pending   bool        `multiIndex:"byOwner,orderedNonUnique"`
	Owner     AccountName `multiIndex:"byOwner,orderedNonUnique"`
	NetWeight int64
	CpuWeight int64
	RamBytes  int64
}

func logObj(data interface{}) {
	space := "	"
	ref := reflect.ValueOf(data)
	if !ref.IsValid() || reflect.Indirect(ref).Kind() != reflect.Struct {
		fmt.Println("log obj valid")
		return
	}

	s := &ref
	if s.Kind() == reflect.Ptr {
		e := s.Elem()
		s = &e
	}
	if s.Kind() != reflect.Struct {
		fmt.Println("log obj valid")
		return
	}
	typ := s.Type()

	num := s.NumField()
	for i := 0; i < num; i++ {
		v := s.Field(i)
		t := typ.Field(i)
		fmt.Print(t.Name, space, v, space)
	}
	fmt.Println("")
}

package testkit

import (
	"fmt"
	"reflect"

	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
)

type Role string

const (
	RoleBootstrapper = Role("bootstrapper")
	RoleMiner        = Role("miner")
	RoleClient       = Role("client")
	RoleDrand        = Role("drand")
	RolePubsubTracer = Role("pubsub-tracer")
)

type RoleConfig struct {
	Role        Role
	PrepareFunc interface{}
	RunFunc     interface{}

	nodeType reflect.Type
}

func (rc *RoleConfig) validate() error {
	var (
		ptyp = reflect.TypeOf(rc.PrepareFunc)
		rtyp = reflect.TypeOf(rc.RunFunc)
	)

	// validate signature of prepare function.
	// it's a function
	ptypvalid := ptyp.Kind() == reflect.Func
	// validate in args
	ptypvalid = ptypvalid && ptyp.NumIn() == 1 && ptyp.In(0) == reflect.TypeOf((*TestEnvironment)(nil))
	// validate out args
	ptypvalid = ptypvalid && ptyp.NumOut() == 2 && ptyp.Out(1) == reflect.TypeOf((error)(nil))
	if !ptypvalid {
		return fmt.Errorf("signature of prepare function is invalid")
	}

	// retain node type
	rc.nodeType = ptyp.Out(0)

	// validate signature of run function.
	rtypvalid := rtyp.Kind() == reflect.Func
	// validate in args
	rtypvalid = rtypvalid && rtyp.NumIn() == 2 && rtyp.In(0) == reflect.TypeOf((*TestEnvironment)(nil)) && rtyp.In(1) == rc.nodeType
	// validate out args
	rtypvalid = rtypvalid && rtyp.NumOut() == 1 && rtyp.Out(0) == reflect.TypeOf((error)(nil))
	if !rtypvalid {
		return fmt.Errorf("signature of run function is invalid")
	}

	return nil
}

var basicRoles = make(map[Role]*RoleConfig)

func MustRegisterRole(role Role, prepareFunc interface{}, runFunc interface{}) {
	if _, ok := basicRoles[role]; ok {
		panic(fmt.Errorf("duplicate role registration: %s", role))
	}
	rc := &RoleConfig{Role: role, PrepareFunc: prepareFunc, RunFunc: runFunc}
	if err := rc.validate(); err != nil {
		panic(fmt.Errorf("failed to validate role config: %w", err))
	}
	basicRoles[role] = rc
}

func MustOverrideRoleRun(role Role, runFunc interface{}) {
	rc, ok := basicRoles[role]
	if !ok {
		panic(fmt.Errorf("role not registered: %s", role))
	}
	newrc := &RoleConfig{Role: role, PrepareFunc: rc.PrepareFunc, RunFunc: runFunc}
	if err := newrc.validate(); err != nil {
		panic(fmt.Errorf("failed to validate role config: %w", err))
	}
	basicRoles[role] = newrc
}

func ExecuteRole(role Role, runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	rc, ok := basicRoles[role]
	if !ok {
		panic(fmt.Errorf("role not registered: %s", role))
	}

	t := &TestEnvironment{runenv, initCtx}

	prepareFn := reflect.ValueOf(rc.PrepareFunc)
	runFn := reflect.ValueOf(rc.RunFunc)

	out := prepareFn.Call([]reflect.Value{reflect.ValueOf(t)})
	if err := out[1].Interface().(error); err != nil {
		return err
	}

	out = runFn.Call([]reflect.Value{reflect.ValueOf(t), out[0]})
	return out[0].Interface().(error)
}

func init() {
	MustRegisterRole(RoleBootstrapper, prepareBootstrapper, runBootstrapper)
	MustRegisterRole(RoleMiner, prepareMiner, runDefaultMiner)
	MustRegisterRole(RoleClient, prepareClient, runDefaultClient)
	MustRegisterRole(RoleDrand, prepareDrandNode, runDrandNode)
	MustRegisterRole(RolePubsubTracer, preparePubsubTracer, runPubsubTracer)
}

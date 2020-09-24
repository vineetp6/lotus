package lotus

import (
	"context"
	"log"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/conformance/chaos"
	"github.com/filecoin-project/lotus/extern/sector-storage/ffiwrapper"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var (
	BaseFee = abi.NewTokenAmount(100) // TODO make parametrisable through vector.
)

type Driver struct {
	ctx context.Context
}

func NewDriver(ctx context.Context) *Driver {
	return &Driver{ctx: ctx}
}

func (d *Driver) ExecuteMessage(msg *types.Message, preroot cid.Cid, bs blockstore.Blockstore, epoch abi.ChainEpoch) (*vm.ApplyRet, cid.Cid, error) {
	log.Println("execution sanity check")
	cst := cbor.NewCborStore(bs)
	st, err := state.LoadStateTree(cst, preroot)
	if err != nil {
		return nil, cid.Undef, err
	}

	actor, err := st.GetActor(msg.From)
	if err != nil {
		log.Println("from actor not found: ", msg.From)
	} else {
		log.Println("from actor found: ", actor)
	}

	log.Println("creating vm")
	vmOpts := &vm.VMOpts{
		StateBase:      preroot,
		Epoch:          epoch,
		Rand:           &vmRand{},
		Bstore:         bs,
		Syscalls:       mkFakedSigSyscalls(vm.Syscalls(ffiwrapper.ProofVerifier)),
		CircSupplyCalc: nil,
		BaseFee:        BaseFee,
		NtwkVersion: func(context.Context, abi.ChainEpoch) network.Version {
			// TODO: Network upgrade.
			return network.Version0
		},
	}
	lvm, err := vm.NewVM(context.Background(), vmOpts)
	if err != nil {
		return nil, cid.Undef, err
	}
	// need to modify the VM invoker to add the chaos actor
	chainValInvoker := vm.NewInvoker()
	chainValInvoker.Register(chaos.ChaosActorCodeCID, chaos.Actor{}, chaos.State{})
	lvm.SetInvoker(chainValInvoker)
	if err != nil {
		return nil, cid.Undef, err
	}

	log.Println("applying message")
	ret, err := lvm.ApplyMessage(d.ctx, msg)
	if err != nil {
		return nil, cid.Undef, err
	}

	log.Printf("applied message: %+v\n", ret)

	log.Println("flushing")
	root, err := lvm.Flush(d.ctx)
	return ret, root, err
}

package experiment

import (
	"context"
	"github.com/mongodb/mongo-go-driver/mongo"
	"regexp"
)

var (
	DatabaseURL      = "mongodb://localhost:27017"
	DatabaseName     = "experiment_reduced_size"
	TxCollectionName = "transactions"
)

// Enum values for different exception kinds
const (
	NoException = iota
	ExplicitRevert
	DepositOutOfGas
	RunOutOfGas
	CallStackOverflow
	DataStackUnderflow
	DataStackOverflow
	InvalidJumpDestination
	InvalidInstruction
	PrecompiledCallError
	InsufficientBalance
	WritePermissionViolation
	ReturnDataOutOfBound
	ContractAddressCollision
	MaxCodeSizeExceeded
	GasUintOverflow
	EmptyCode
)

type Trace struct {
	CallStackDepth uint16
	Type           string // one in "create", "call", "callcode", "delegatecall", "staticcall"
	From           string
	To             string
	Value          string
	GasLimit       uint32
	GasLeft        uint32 // remaining gas after execution of this step
	StatusCode     uint8
	NewAddress     string // new account address if current transaction is a contract create
	ErrorMsg       string
	ErrorCode      uint8 // 0 for no exception, 1 for explicit exception, 2 and so on for each implicit exception
}

// for exceptional transactions
type Transaction struct {
	BlockNum     uint32
	TxIndex      uint16
	Nonce        uint64
	TxHash       string
	From         string
	To           string
	Value        string
	GasLimit     uint32
	GasPrice     string
	GasUsed      uint32 // gas used during execution of this transaction
	StatusCode   uint8  // external transaction status code
	NumSteps     uint32 // number of execution steps this transaction takes
	HasException bool   // whether this transaction encounters any form of exception (including internal ones)
	Traces       []*Trace
}

func NewTxRecord() *Transaction {
	txRecord := new(Transaction)
	txRecord.Traces = make([]*Trace, 0)
	return txRecord
}

// Create a new Steps instance, insert into slices, return a pointer of it
func (tx *Transaction) NewTrace() *Trace {
	trace := new(Trace)
	tx.Traces = append(tx.Traces, trace)
	return trace
}

func (tx *Transaction) ReleaseInternal() {
	for i := range tx.Traces {
		tx.Traces[i] = nil // mark each trace pointer as nil (for garbage collection)
	}
}

func CheckException(err error) (exception string, kind uint8) {
	if err == nil {
		return "", NoException // no exception
	}

	switch {
	case err.Error() == "evm: execution reverted": // explicit exception
		return err.Error(), ExplicitRevert
	case err.Error() == "contract creation code storage out of gas": // out of code deposit gas
		return err.Error(), DepositOutOfGas
	case err.Error() == "out of gas": // out of runtime gas
		return err.Error(), RunOutOfGas
	case err.Error() == "max call depth exceeded": // call stack overflow
		return err.Error(), CallStackOverflow
	case len(regexp.MustCompile("^stack underflow .+$").FindAllString(err.Error(), -1)) > 0: // data stack underflow
		return err.Error(), DataStackUnderflow
	case len(regexp.MustCompile("^stack limit reached .+$").FindAllString(err.Error(), -1)) > 0: // data stack overflow
		return err.Error(), DataStackOverflow
	case len(regexp.MustCompile("^invalid jump destination .+$").FindAllString(err.Error(), -1)) > 0: // invalid jump destination
		return err.Error(), InvalidJumpDestination
	case len(regexp.MustCompile("^invalid opcode .+$").FindAllString(err.Error(), -1)) > 0: // invalid instruction
		return err.Error(), InvalidInstruction
	case err.Error() == "insufficient balance for transfer": // insufficient balance
		return err.Error(), InsufficientBalance
	case err.Error() == "evm: write protection": // write permission violation
		return err.Error(), WritePermissionViolation
	case err.Error() == "evm: return data out of bounds": // return data out of bound
		return err.Error(), ReturnDataOutOfBound
		//case vm.ErrTraceLimitReached.ErrorMsg():
		//	break
	case err.Error() == "contract address collision": // contract address collision
		return err.Error(), ContractAddressCollision
		//case vm.ErrNoCompatibleInterpreter.ErrorMsg():
		//	break
	case err.Error() == "evm: max code size exceeded": // max code size exceeded
		return err.Error(), MaxCodeSizeExceeded
	case err.Error() == "gas uint64 overflow": // gas overflow (beyond reach of uint64 type)
		return err.Error(), GasUintOverflow
	case err.Error() == "empty call code": // call to an empty code (exclude pure value transfer)
		return err.Error(), EmptyCode
	default: // precompiled contract call exception
		return err.Error(), PrecompiledCallError
	}
}

func Collections() (collTx *mongo.Collection, err error) {
	client, err := mongo.Connect(context.Background(), DatabaseURL)
	if err != nil {
		return nil, err
	}
	db := client.Database(DatabaseName)
	collTx = db.Collection(TxCollectionName)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return collTx, nil
}

func CloseConnection(coll *mongo.Collection) (err error) {
	return coll.Database().Client().Disconnect(context.Background())
}

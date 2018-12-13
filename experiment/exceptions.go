package experiment

import (
	"context"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/gridfs"
	"github.com/mongodb/mongo-go-driver/options"
	"regexp"
)

var (
	DatabaseURL             = "mongodb://localhost:27017"
	DatabaseName            = "experiment"
	ExceptionCollectionName = "exceptions"
	CodeCollectionName      = "contract_code"
	TxCollectionName        = "transactions"
	BucketName              = "exception_bucket"
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

type OneStep struct {
	StepNum      uint64 // Steps step number (i.e. consecutive numbers starting from 1)
	PC           uint64
	Ins          string
	RemainingGas uint64 // remaining gas after execution of this step
}

type Trace struct {
	CallStackDepth uint64
	From           string
	To             string
	Value          string
	Input          string
	GasLimit       uint64
	GasPrice       string
	StatusCode     uint64
	ErrorMsg       string
	// TODO Define different kind for each explicit exception
	ErrorCode  uint64 // 0 for no exception, 1 for explicit exception, 2 and so on for each implicit exception
	Steps      []*OneStep
	TraceDocID string // refer to a GridFS file in case ExceptionalTx being too large
}

// for exceptional transactions
type ExceptionalTx struct {
	BlockNum      uint64
	TxIndex       uint64
	Nonce         uint64
	TxHash        string
	From          string
	To            string
	Value         string
	Input         string
	GasLimit      uint64
	GasPrice      string
	CreateAddress string // new account address if current transaction is a contract create
	StatusCode    uint64 // external transaction status code
	NumSteps      uint64 // number of execution steps this transaction takes
	HasException  bool   // whether this transaction encounters any form of exception (including internal ones)
	Traces        []*Trace
}

// for all transactions
type TxInfo struct {
	BlockNum      uint64
	TxIndex       uint64
	Nonce         uint64
	TxHash        string
	From          string
	To            string
	Value         string
	Input         string
	GasPrice      string
	GasLimit      uint64
	CreateAddress string // new account address if current transaction is a contract create
	External      bool   // whether this is an external transaction
}

type ContractCode struct {
	Address  string
	ByteCode string // runtime contract code (after initialization)
	Nonce    uint64 // nonce used to generate this contract's address
	From     string // account that create this contract
	Value    string // initial deposit value
	GasLimit uint64 // initial gas limit
	GasPrice string // initial gas price
	Input    string // initial contract code (including construction)
	TxHash   string // transaction hash of the external transaction
	External bool   // whether this account for an external transaction
}

func NewTxRecord() *ExceptionalTx {
	txRecord := new(ExceptionalTx)
	txRecord.Traces = make([]*Trace, 0)
	return txRecord
}

// Create a new Steps instance, insert into slices, return a pointer of it
func (tx *ExceptionalTx) NewTrace() *Trace {
	trace := new(Trace)
	trace.Steps = make([]*OneStep, 0)
	tx.Traces = append(tx.Traces, trace)
	return trace
}

func (tx *ExceptionalTx) ReleaseInternal() {
	for i, p := range tx.Traces {
		p.ReleaseInternal() // mark each trace step pointer as nil (for garbage collection)
		tx.Traces[i] = nil  // mark each trace pointer as nil (for garbage collection)
	}
}

func (t *Trace) ReleaseInternal() {
	for i := range t.Steps {
		t.Steps[i] = nil // mark trace step pointer as nil (for garbage collection)
	}
}

func CheckException(err error) (exception string, kind uint64) {
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

func Collections() (collException *mongo.Collection, collCode *mongo.Collection, collTx *mongo.Collection, gridfsbucket *gridfs.Bucket, err error) {
	client, err := mongo.Connect(context.Background(), DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	db := client.Database(DatabaseName)
	collException = db.Collection(ExceptionCollectionName)
	collCode = db.Collection(CodeCollectionName)
	collTx = db.Collection(TxCollectionName)
	gridfsbucket, err = gridfs.NewBucket(db, &options.BucketOptions{Name: &BucketName})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return collException, collCode, collTx, gridfsbucket, nil
}

func CloseConnection(coll *mongo.Collection) (err error) {
	return coll.Database().Client().Disconnect(context.Background())
}

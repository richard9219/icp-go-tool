package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	agent "github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/candid/idl"
	"github.com/aviate-labs/agent-go/ic"
	"github.com/aviate-labs/agent-go/ic/icpledger"
	"github.com/aviate-labs/agent-go/ic/wallet"
	"github.com/aviate-labs/agent-go/identity"
	"github.com/aviate-labs/agent-go/principal"
)

type (
	Account struct {
		Account string `ic:"account"`
	}

	Balance struct {
		E8S uint64 `ic:"e8s"`
	}
)

func savePEMToFile(pemData []byte, filename string) error {
	return os.WriteFile(filename, pemData, 0644)
}

func GenWallet() (string, string, error) {
	id, error1 := identity.NewRandomSecp256k1Identity()
	if error1 != nil {
		return "", "", error1
	}

	principalID := id.Sender()
	accountID := principal.NewAccountID(principalID, [32]byte{})

	pemData, error2 := id.ToPEM()
	if error2 != nil {
		return "", "", error2
	}

	filename := id.Sender().String() + ".pem"
	error3 := savePEMToFile(pemData, filename)

	if error3 != nil {
		return "", "", error3
	}

	return principalID.String(), accountID.String(), nil
}

func GenBatchWallet(num int) {
	file, err := os.Create("account.txt")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}

	defer file.Close()

	for i := 0; i <= num; i++ {
		if i == 0 {
			line := ("num, principalID, accountID, balance\n")
			if _, err := file.WriteString(line); err != nil {
				fmt.Println("Error writing to file:", err)
			}
		} else {
			principalID, accountID, error := GenWallet()
			if error == nil {
				line := fmt.Sprintf("%d, %s, %s, %f\n", i, principalID, accountID, QueryAccountBalance(accountID))
				if _, err := file.WriteString(line); err != nil {
					fmt.Println("No", i, "times gen wallet, error writing to file:", err)
				}
			}
		}
	}
}

func QueryAccountBalance(accountID string) float64 {
	var balance Balance
	a, _ := agent.New(agent.DefaultConfig)
	a.Query(
		ic.LEDGER_PRINCIPAL, "account_balance_dfx",
		[]any{Account{accountID}},
		[]any{&balance},
	)
	return float64(balance.E8S) / 1e8
}

func UpdateBatchAccountFileBalance() {
	fileName := "account.txt"

	file, err := os.Open(fileName)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	for i, line := range lines {
		if i == 0 {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			fmt.Println("Invalid line format:", line)
			continue
		}

		accountID := strings.TrimSpace(parts[2])
		newBalance := QueryAccountBalance(accountID)

		parts[3] = fmt.Sprintf("%.6f", newBalance)
		lines[i] = strings.Join(parts, ",")
	}

	err = os.WriteFile(fileName, []byte(strings.Join(lines, "\n")), 0644)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	fmt.Println("Balances updated successfully!")
}

func loadPem(principalID string) (*identity.Secp256k1Identity, error) {
	filename := principalID + ".pem"
	data, err1 := os.ReadFile(filename)
	if err1 != nil {
		return nil, err1
	}
	id, err2 := identity.NewSecp256k1IdentityFromPEM(data)
	if err2 != nil {
		return nil, err2
	}
	return id, nil
}

func CallCanisterMethod(callerPrincipalID, networkURL, walletCanister, callCanister, callMethod string, arg []any) (*wallet.WalletResultCall, error) {
	id, err1 := loadPem(callerPrincipalID)
	if err1 != nil {
		return nil, err1
	}
	principalID := id.Sender()
	fmt.Println("CallerPrincipalID", principalID)
	fmt.Println("CallerAccountID", principal.NewAccountID(principalID, [32]byte{}))
	fmt.Println("CallerAccountBalance", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))
	fmt.Println("WalletCanister", walletCanister)
	fmt.Println("CallCanister", callCanister)
	fmt.Println("CallMethod", callMethod)

	host, err2 := url.Parse(networkURL)
	if err2 != nil {
		return nil, err2
	}

	cfg := agent.Config{
		Identity:                       id,
		ClientConfig:                   &agent.ClientConfig{Host: host},
		FetchRootKey:                   true,
		PollTimeout:                    30 * time.Second,
		DisableSignedQueryVerification: false, //must be true for access local network
	}

	//identity's wallet-canister, get by "dfx identity get-wallet"
	a, err3 := wallet.NewAgent(principal.MustDecode(walletCanister), cfg)

	if err3 != nil {
		return nil, err3
	}

	inputArgs, err4 := idl.Marshal(arg) //format parameters to []byte
	if err4 != nil {
		return nil, err4
	}

	arg0 := struct {
		Canister   principal.Principal `ic:"canister" json:"canister"`
		MethodName string              `ic:"method_name" json:"method_name"`
		Args       []byte              `ic:"args" json:"args"`
		Cycles     uint64              `ic:"cycles" json:"cycles"`
	}{
		Canister:   principal.MustDecode(callCanister),
		MethodName: callMethod,
		Args:       inputArgs,
		Cycles:     100_000_000,
	}

	res, err5 := a.WalletCall(arg0)
	if err5 != nil {
		return nil, err5
	}
	return res, nil
}

type Spender struct {
	Owner      principal.Principal `ic:"owner" json:"owner"`
	Subaccount idl.Null            `ic:"subaccount" json:"subaccount"`
}

type CallArgs struct {
	Fee               idl.Null `ic:"fee" json:"fee"`
	Memo              idl.Null `ic:"memo" json:"memo"`
	FromSubaccount    idl.Null `ic:"from_subaccount" json:"from_subaccount"`
	CreatedAtTime     idl.Null `ic:"created_at_time" json:"created_at_time"`
	Amount            idl.Nat  `ic:"amount" json:"amount"`
	ExpectedAllowance idl.Null `ic:"expected_allowance" json:"expected_allowance"`
	ExpiresAt         idl.Null `ic:"expires_at" json:"expires_at"`
	Spender           Spender  `ic:"spender" json:"spender"`
}

type GenericError struct {
	Message   string  `ic:"message"`
	ErrorCode idl.Nat `ic:"error_code"`
}

type Duplicate struct {
	DuplicateOf idl.Nat `ic:"duplicate_of"`
}

type BadFee struct {
	ExpectedFee idl.Nat `ic:"expected_fee"`
}

type AllowanceChanged struct {
	CurrentAllowance idl.Nat `ic:"current_allowance"`
}

type CreatedInFuture struct {
	LedgerTime uint64 `ic:"ledger_time"`
}

type Expired struct {
	LedgerTime uint64 `ic:"ledger_time"`
}

type InsufficientFunds struct {
	Balance idl.Nat `ic:"balance"`
}

type ApproveError struct {
	GenericError           *GenericError      `ic:"GenericError" json:"GenericError"`
	TemporarilyUnavailable bool               `ic:"TemporarilyUnavailable" json:"TemporarilyUnavailable"`
	Duplicate              *Duplicate         `ic:"Duplicate" json:"Duplicate"`
	BadFee                 *BadFee            `ic:"BadFee" json:"BadFee"`
	AllowanceChanged       *AllowanceChanged  `ic:"AllowanceChanged" json:"AllowanceChanged"`
	CreatedInFuture        *CreatedInFuture   `ic:"CreatedInFuture" json:"CreatedInFuture"`
	TooOld                 bool               `ic:"TooOld" json:"TooOld"`
	Expired                *Expired           `ic:"Expired" json:"Expired"`
	InsufficientFunds      *InsufficientFunds `ic:"InsufficientFunds" json:"InsufficientFunds"`
}

type Result struct {
	Ok  *idl.Nat      `ic:"Ok"`
	Err *ApproveError `ic:"Err"`
}

const CANISTER_ID_ICPL_ROUTER = "2ackz-dyaaa-aaaam-ab5eq-cai"
const CANISTER_ID_WALLET_DEV = "wwon2-uyaaa-aaaai-aqlyq-cai"
const CANISTER_ID_WCP = "pmvba-2aaaa-aaaam-adyta-cai"
const CANISTER_ID_ICP_WCP_POOL = "pluhu-xyaaa-aaaam-adytq-cai"

func CallLocalCanister() {
	// local hello world Canister Call
	// CallCanisterMethod("test", "http://127.0.0.1:4943/", "be2us-64aaa-aaaaa-qaabq-cai", "br5f7-7uaaa-aaaaa-qaaca-cai", "getMessage", "")

}

func CallQuery() {
	// IC Canister Call
	inputArgs := []any{nil}
	resp, err := CallCanisterMethod("dev", "https://icp-api.io/", "wwon2-uyaaa-aaaai-aqlyq-cai", "2ackz-dyaaa-aaaam-ab5eq-cai", "cycles", inputArgs)
	if err != nil {
		fmt.Println("CallCanister error ", resp)
	} else {
		if resp.Err != nil {
			fmt.Println("CallCanister Resp error", *resp.Err)
		} else {
			var s idl.Nat
			idl.Unmarshal(resp.Ok.Return, []any{&s})
			fmt.Println("CallCanister Result", s)
		}
	}
}

func CallAproveICP(senderPem, spenderCanister string, approveBalance uint) {
	// Fomoewell Canister Call-approve
	id, _ := loadPem(senderPem)

	cfg := agent.Config{
		Identity: id,
	}

	a, err := icpledger.NewAgent(ic.LEDGER_PRINCIPAL, cfg)
	if err != nil {
		fmt.Println("icpledger.NewAgent err", err)
	}

	spender := icpledger.Account{
		Owner:      principal.MustDecode(spenderCanister),
		Subaccount: nil,
	}
	callArgsParam := icpledger.ApproveArgs{
		FromSubaccount:    nil,
		Spender:           spender,
		Amount:            idl.NewNat(approveBalance),
		ExpectedAllowance: nil,
		ExpiresAt:         nil,
		Fee:               nil,
		Memo:              nil,
		CreatedAtTime:     nil,
	}
	result, err := a.Icrc2Approve(callArgsParam)
	if err != nil {
		fmt.Println("icpledger.Icrc2Approve err", err)
	}

	if result.Ok != nil {
		fmt.Println("icpledger.Icrc2Approve Success, tx index", result.Ok)
	} else if result.Err != nil {
		if result.Err.GenericError != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: GenericError - %s (Code: %d)\n",
				result.Err.GenericError.Message, result.Err.GenericError.ErrorCode)
		} else if result.Err.TemporarilyUnavailable != nil {
			fmt.Println("icpledger.Icrc2Approve Error: TemporarilyUnavailable")
		} else if result.Err.Duplicate != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: Duplicate - DuplicateOf: %d\n",
				result.Err.Duplicate.DuplicateOf)
		} else if result.Err.BadFee != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: BadFee - ExpectedFee: %d\n",
				result.Err.BadFee.ExpectedFee)
		} else if result.Err.AllowanceChanged != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: AllowanceChanged - CurrentAllowance: %d\n",
				result.Err.AllowanceChanged.CurrentAllowance)
		} else if result.Err.CreatedInFuture != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: CreatedInFuture - LedgerTime: %d\n",
				result.Err.CreatedInFuture.LedgerTime)
		} else if result.Err.TooOld != nil {
			fmt.Println("icpledger.Icrc2Approve Error: TooOld")
		} else if result.Err.Expired != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: Expired - LedgerTime: %d\n",
				result.Err.Expired.LedgerTime)
		} else if result.Err.InsufficientFunds != nil {
			fmt.Printf("icpledger.Icrc2Approve Error: InsufficientFunds - Balance: %d\n",
				result.Err.InsufficientFunds.Balance)
		} else {
			fmt.Println("icpledger.Icrc2Approve Error: Unknown Error")
		}
	} else {
		fmt.Println("icpledger.Icrc2Approve Unknown Response")
	}
}

func CallAproveICRC(senderPem, callerCanister, spenderCanister string, approveBalance uint) {
	// Fomoewell Canister Call-approve
	id, _ := loadPem(senderPem)
	principalID := id.Sender()
	fmt.Println("CallerPrincipalID", principalID)
	fmt.Println("CallerAccountID", principal.NewAccountID(principalID, [32]byte{}))
	fmt.Println("CallerAccountBalance before", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))

	cfg := agent.Config{
		Identity: id,
	}
	a, _ := agent.New(cfg)

	spender := icpledger.Account{
		Owner:      principal.MustDecode(spenderCanister),
		Subaccount: nil,
	}
	callArgsParam := icpledger.ApproveArgs{
		FromSubaccount:    nil,
		Spender:           spender,
		Amount:            idl.NewNat(approveBalance),
		ExpectedAllowance: nil,
		ExpiresAt:         nil,
		Fee:               nil,
		Memo:              nil,
		CreatedAtTime:     nil,
	}

	var result icpledger.ApproveResult
	if err := a.Call(
		principal.MustDecode(callerCanister),
		"icrc2_approve",
		[]any{callArgsParam},
		[]any{&result},
	); err != nil {
		fmt.Println("Icrc2Approve Success, tx index", result.Ok)
		return
	}

	if result.Ok != nil {
		fmt.Println("Icrc2Approve Success, tx index", result.Ok)
	} else if result.Err != nil {
		if result.Err.GenericError != nil {
			fmt.Printf("Icrc2Approve Error: GenericError - %s (Code: %d)\n",
				result.Err.GenericError.Message, result.Err.GenericError.ErrorCode)
		} else if result.Err.TemporarilyUnavailable != nil {
			fmt.Println("Icrc2Approve Error: TemporarilyUnavailable")
		} else if result.Err.Duplicate != nil {
			fmt.Printf("Icrc2Approve Error: Duplicate - DuplicateOf: %d\n",
				result.Err.Duplicate.DuplicateOf)
		} else if result.Err.BadFee != nil {
			fmt.Printf("Icrc2Approve Error: BadFee - ExpectedFee: %d\n",
				result.Err.BadFee.ExpectedFee)
		} else if result.Err.AllowanceChanged != nil {
			fmt.Printf("Icrc2Approve Error: AllowanceChanged - CurrentAllowance: %d\n",
				result.Err.AllowanceChanged.CurrentAllowance)
		} else if result.Err.CreatedInFuture != nil {
			fmt.Printf("Icrc2Approve Error: CreatedInFuture - LedgerTime: %d\n",
				result.Err.CreatedInFuture.LedgerTime)
		} else if result.Err.TooOld != nil {
			fmt.Println("Icrc2Approve Error: TooOld")
		} else if result.Err.Expired != nil {
			fmt.Printf("Icrc2Approve Error: Expired - LedgerTime: %d\n",
				result.Err.Expired.LedgerTime)
		} else if result.Err.InsufficientFunds != nil {
			fmt.Printf("Icrc2Approve Error: InsufficientFunds - Balance: %d\n",
				result.Err.InsufficientFunds.Balance)
		} else {
			fmt.Println("Icrc2Approve Error: Unknown Error")
		}
	} else {
		fmt.Println("Icrc2Approve Unknown Response")
	}
}

func CallSwapTokenToToken(senderPem, callCanister, baseFromTokenCanister, baseToTokenCanister, poolCanister string, baseFromAmount, baseMinReturnAmount, directions uint) {

	id, _ := loadPem(senderPem)
	principalID := id.Sender()
	fmt.Println("CallerPrincipalID", principalID)
	fmt.Println("CallerAccountID", principal.NewAccountID(principalID, [32]byte{}))
	fmt.Println("CallerAccountBalance before", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))
	fmt.Println("CallCanister", callCanister)

	cfg := agent.Config{
		Identity: id,
	}
	a, _ := agent.New(cfg)

	arg := []interface{}{
		principal.MustDecode(baseFromTokenCanister),                           // baseFromTokenCanister (principal)
		principal.MustDecode(baseToTokenCanister),                             // baseToTokenCanister (principal)
		idl.NewNat(baseFromAmount),                                            // baseFromAmount (nat)
		idl.NewNat(baseMinReturnAmount),                                       // base_min_return_amount (nat)
		[]principal.Principal{principal.MustDecode(CANISTER_ID_ICP_WCP_POOL)}, // pairs (vec principal)
		uint64(directions),                                                    // Offset (nat64)
		uint64(time.Now().UnixMilli()*1e6 + 5*60*1e9),                         // Timeout (nat64)
	}

	type SwapResult struct {
		Ok  *uint64 `ic:"Ok"`
		Err *[]byte `ic:"Err"`
	}

	var result SwapResult
	if err := a.Call(
		principal.MustDecode(callCanister),
		"swapTokenToToken",
		arg,
		[]any{&result},
	); err != nil {
		fmt.Println("Call ICPL SwapTokenToToken Call Error:", err)
		return
	}

	if result.Ok != nil {
		fmt.Println("Call ICPL SwapTokenToToken Success: ", *result.Ok)
	} else if result.Err != nil {
		fmt.Println("Call ICPL SwapTokenToToken Error:", *result.Err)
	} else {
		fmt.Println("Call ICPL SwapTokenToToken unkown resp:", result)
	}

	fmt.Println("CallerAccountBalance after", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))
}

func CallAddLiquidity(senderPem, callCanister string) {

	id, _ := loadPem(senderPem)
	principalID := id.Sender()
	fmt.Println("CallerPrincipalID", principalID)
	fmt.Println("CallerAccountID", principal.NewAccountID(principalID, [32]byte{}))
	fmt.Println("CallerAccountBalance before", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))
	fmt.Println("CallCanister", callCanister)

	cfg := agent.Config{
		Identity: id,
	}
	a, _ := agent.New(cfg)

	arg := []interface{}{
		principal.MustDecode(CANISTER_ID_ICP_WCP_POOL),                // pool (principal)
		idl.NewNat(uint(10_000_000_000 + 100_000_000)),                // tokenICPAmount (nat)
		idl.NewNat(uint(73_066_057_177_319_0000 + 1_000_000_000_000)), // tokenICRC20Amount (nat)
		idl.NewNat(uint(1_0000_000_000_000_000_000)),                  // slippage (nat)
		uint64(time.Now().UnixMilli()*1e6 + 5*60*1e9),                 // Timeout (nat64)
	}
	type LPResult struct {
		LP     *idl.Nat `ic:"lp"`
		TokenA *idl.Nat `ic:"token_a"`
		TokenB *idl.Nat `ic:"token_b"`
	}
	type AddLPResult struct {
		Ok  *LPResult `ic:"Ok"`
		Err *[]byte   `ic:"Err"`
	}

	var result AddLPResult
	if err := a.Call(
		principal.MustDecode(callCanister),
		"addLiquidity",
		arg,
		[]any{&result},
	); err != nil {
		fmt.Println("Call ICPL SwapTokenToToken Call Error:", err)
		return
	}

	if result.Ok != nil {
		fmt.Println("Call ICPL SwapTokenToToken Success: get swaptoken", *result.Ok)
	} else if result.Err != nil {
		fmt.Println("Call ICPL SwapTokenToToken Error:", *result.Err)
	} else {
		fmt.Println("Call ICPL SwapTokenToToken unkown resp:", result)
	}

	fmt.Println("CallerAccountBalance after", QueryAccountBalance(principal.NewAccountID(principalID, [32]byte{}).String()))
}

func main() {

	// batch gen wallet
	// GenBatchWallet(10)

	// batch wallet balance udpate
	// UpdateBatchAccountFileBalance()

	//CallLocalCanister()

	// CallAproveICP("dev", CANISTER_ID_ICPL_ROUTER, 10_000_000) // = CallAproveICRC("dev", ic.LEDGER_PRINCIPAL.String(), CANISTER_ID_ICPL_ROUTER, 10_000_000)

	// CallAproveICRC("dev", CANISTER_ID_WCP, CANISTER_ID_ICPL_ROUTER, 10_000_000)

	// CallSwapTokenToToken("dev", CANISTER_ID_ICPL_ROUTER, ic.LEDGER_PRINCIPAL.String(), CANISTER_ID_WCP, CANISTER_ID_ICP_WCP_POOL, 1_000_000, 0, 1)

	// CallAddLiquidity("dev", CANISTER_ID_ICPL_ROUTER)

}

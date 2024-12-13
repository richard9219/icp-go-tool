package main

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aviate-labs/agent-go"
	"github.com/aviate-labs/agent-go/candid/idl"
	"github.com/aviate-labs/agent-go/ic"
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

	fmt.Println("pemData", pemData)

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

func CallCanisterMethod(callerPrincipalID, networkURL, subnetCanister, callCanister, callMethod, callParam string) {
	id, _ := loadPem(callerPrincipalID)
	principalID := id.Sender()
	fmt.Println("callerPrincipalID", principalID)
	fmt.Println("callerAccountID", principal.NewAccountID(principalID, [32]byte{}))
	fmt.Println("subnetCanister", subnetCanister)
	fmt.Println("callCanister", callCanister)
	fmt.Println("callMethod", callMethod)
	fmt.Println("callParam", callParam)

	host, _ := url.Parse(networkURL)

	cfg := agent.Config{
		Identity:                       id,
		ClientConfig:                   &agent.ClientConfig{Host: host},
		FetchRootKey:                   true,
		PollTimeout:                    30 * time.Second,
		DisableSignedQueryVerification: false, //must be true for access local network
	}

	//identity's wallet-canister, get by "dfx identity get-wallet"
	a, _ := wallet.NewAgent(principal.MustDecode(subnetCanister), cfg)

	input, _ := idl.Marshal([]any{callParam}) //format parameters to []byte

	arg0 := struct {
		Canister   principal.Principal `ic:"canister" json:"canister"`
		MethodName string              `ic:"method_name" json:"method_name"`
		Args       []byte              `ic:"args" json:"args"`
		Cycles     uint64              `ic:"cycles" json:"cycles"`
	}{
		Canister:   principal.MustDecode(callCanister),
		MethodName: callMethod,
		Args:       []byte(input),
		Cycles:     200_000_000,
	}

	res, _ := a.WalletCall(arg0)
	fmt.Printf("res:%v\n", res)
	var s string
	idl.Unmarshal(res.Ok.Return, []any{&s})
	fmt.Printf("result:%v\n", s)
}

func main() {

	// batch gen wallet
	// GenBatchWallet(10)

	// batch wallet balance udpate
	// UpdateBatchAccountFileBalance()

	// local hello world Canister Call
	// CallCanisterMethod("xmdfw-fnaby-bthxw-wedg2-q5jfr-fw2pm-25zxg-5w7s4-cofkn-gfzzj-4ae", "http://127.0.0.1:4943/", "be2us-64aaa-aaaaa-qaabq-cai", "br5f7-7uaaa-aaaaa-qaaca-cai", "getMessage", "")

	// IC Canister Call
	// CallCanisterMethod("xmdfw-fnaby-bthxw-wedg2-q5jfr-fw2pm-25zxg-5w7s4-cofkn-gfzzj-4ae", "https://icp-api.io/", "3dod7-xaaaa-aaaam-ab5ca-cai", "2ackz-dyaaa-aaaam-ab5eq-cai", "getPoolsInfo", "")
}

package funding

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const erc20ABI = `[{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"value","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"},{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"}]`
const extendedBridgeABI = `[{"inputs":[{"internalType":"address","name":"token","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"uint256","name":"commitmentId","type":"uint256"}],"name":"depositWithId","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
const asterTreasuryABI = `[{"inputs":[{"internalType":"address","name":"currency","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"uint256","name":"broker","type":"uint256"}],"name":"deposit","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
const defaultExtendedBaseURL = "https://api.starknet.extended.exchange"
const defaultAsterArbitrumTreasury = "0x9E36CB86a159d479cEd94Fa05036f235Ac40E1d5"

type EVMDepositor struct {
	HTTPClient *http.Client
}

func (d EVMDepositor) Deposit(ctx context.Context, plan DepositPlan) (DepositResult, error) {
	result := DepositResult{
		AccountName: plan.Account.Name,
		Venue:       plan.Balance.Venue,
		AmountUSDC:  plan.Amount,
		DryRun:      plan.DryRun,
		Route:       plan.Account.Deposit.Type,
		CompletedAt: time.Now().UTC(),
	}
	if plan.DryRun {
		result.Status = "dry_run"
		result.Message = "deposit skipped by dry_run"
		return result, nil
	}
	switch strings.TrimSpace(plan.Account.Deposit.Type) {
	case "evm_usdc_transfer":
		txHash, err := d.transferUSDC(ctx, plan, plan.Account.Deposit.ToAddress)
		result.TxHash = txHash
		if err != nil {
			result.Status = "error"
			return result, err
		}
		result.Status = "success"
		return result, nil
	case "lighter_cctp_intent":
		intentAddress, metadata, err := d.lighterIntentAddress(ctx, plan)
		if err != nil {
			result.Status = "error"
			return result, err
		}
		txHash, err := d.transferUSDC(ctx, plan, intentAddress)
		result.TxHash = txHash
		result.Metadata = metadata
		result.Metadata["intent_address"] = intentAddress
		if err != nil {
			result.Status = "error"
			return result, err
		}
		result.Status = "success"
		return result, nil
	case "extended_rhino_bridge":
		txHash, metadata, err := d.extendedRhinoBridge(ctx, plan)
		result.TxHash = txHash
		result.Metadata = metadata
		if err != nil {
			result.Status = "error"
			return result, err
		}
		result.Status = "success"
		return result, nil
	case "aster_treasury_deposit":
		txHash, metadata, err := d.asterTreasuryDeposit(ctx, plan)
		result.TxHash = txHash
		result.Metadata = metadata
		if err != nil {
			result.Status = "error"
			return result, err
		}
		result.Status = "success"
		return result, nil
	case "command":
		return d.commandDeposit(ctx, plan, result)
	case "":
		return result, fmt.Errorf("deposit.type is required")
	default:
		return result, fmt.Errorf("unsupported deposit.type %q", plan.Account.Deposit.Type)
	}
}

func (d EVMDepositor) commandDeposit(ctx context.Context, plan DepositPlan, result DepositResult) (DepositResult, error) {
	if len(plan.Account.Deposit.Command) == 0 {
		return result, fmt.Errorf("command deposit requires deposit.command")
	}
	input, err := json.Marshal(plan)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, plan.Account.Deposit.Command[0], plan.Account.Deposit.Command[1:]...)
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return result, fmt.Errorf("command deposit failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if strings.TrimSpace(out.String()) == "" {
		result.Status = "success"
		result.Message = strings.TrimSpace(stderr.String())
		return result, nil
	}
	var decoded DepositResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		return result, fmt.Errorf("decode command deposit result: %w", err)
	}
	if decoded.AccountName == "" {
		decoded.AccountName = result.AccountName
	}
	if decoded.Venue == "" {
		decoded.Venue = result.Venue
	}
	if decoded.AmountUSDC == 0 {
		decoded.AmountUSDC = result.AmountUSDC
	}
	if decoded.Route == "" {
		decoded.Route = result.Route
	}
	if decoded.CompletedAt.IsZero() {
		decoded.CompletedAt = time.Now().UTC()
	}
	if decoded.Status == "" {
		decoded.Status = "success"
	}
	return decoded, nil
}

func (d EVMDepositor) transferUSDC(ctx context.Context, plan DepositPlan, to string) (string, error) {
	if !common.IsHexAddress(to) {
		return "", fmt.Errorf("invalid destination address %q", to)
	}
	wallet := resolveWallet(plan)
	rpcURL := envValue(wallet.RPCURLEnv)
	if rpcURL == "" {
		return "", fmt.Errorf("missing %s", wallet.RPCURLEnv)
	}
	keyRaw := envValue(wallet.PrivateKeyEnv)
	if keyRaw == "" {
		return "", fmt.Errorf("missing %s", wallet.PrivateKeyEnv)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyRaw, "0x"))
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", wallet.PrivateKeyEnv, err)
	}
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return "", err
	}
	defer client.Close()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return "", err
	}
	if wallet.ChainID > 0 && chainID.Int64() != wallet.ChainID {
		return "", fmt.Errorf("rpc chain id %s does not match configured chain id %d", chainID.String(), wallet.ChainID)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", err
	}
	amount, err := usdcAmount(plan.Amount)
	if err != nil {
		return "", err
	}
	token := common.HexToAddress(wallet.USDCAddress)
	balance, err := erc20Balance(ctx, client, token, from, parsedABI)
	if err != nil {
		return "", fmt.Errorf("check USDC balance: %w", err)
	}
	if balance.Cmp(amount) < 0 {
		return "", fmt.Errorf("%s has insufficient USDC: need %s, have %s", wallet.PrivateKeyEnv, amount.String(), balance.String())
	}
	data, err := parsedABI.Pack("transfer", common.HexToAddress(to), amount)
	if err != nil {
		return "", err
	}
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", err
	}
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &token, Value: big.NewInt(0), Data: data})
	if err != nil {
		return "", fmt.Errorf("estimate gas: %w", err)
	}
	gasTip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		gasTip = big.NewInt(100_000_000)
	}
	paddedGasLimit := uint64(math.Ceil(float64(gasLimit) * 1.20))
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", err
	}
	tx, err := buildERC20TransferTx(ctx, client, header, chainID, nonce, token, data, gasTip, paddedGasLimit, from)
	if err != nil {
		return "", err
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		return "", err
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, client, signed)
	if err != nil {
		return signed.Hash().Hex(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return signed.Hash().Hex(), fmt.Errorf("USDC transfer reverted")
	}
	return signed.Hash().Hex(), nil
}

func (d EVMDepositor) extendedRhinoBridge(ctx context.Context, plan DepositPlan) (string, map[string]any, error) {
	wallet := resolveWallet(plan)
	if plan.Account.Deposit.TokenAddress != "" {
		wallet.USDCAddress = plan.Account.Deposit.TokenAddress
	}
	apiKeyEnv := strings.TrimSpace(plan.Account.Deposit.APIKeyEnv)
	apiKey := envValue(apiKeyEnv)
	if apiKey == "" {
		return "", nil, fmt.Errorf("missing %s", apiKeyEnv)
	}
	baseURL := strings.TrimRight(plan.Account.Deposit.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultExtendedBaseURL
	}
	chainIn := strings.TrimSpace(plan.Account.Deposit.BridgeChainIn)
	if chainIn == "" {
		chainIn = "ARB"
	}
	chainOut := strings.TrimSpace(plan.Account.Deposit.BridgeChainOut)
	if chainOut == "" {
		chainOut = "STRK"
	}
	asset := strings.TrimSpace(plan.Account.Deposit.BridgeAsset)
	if asset == "" {
		asset = "USD"
	}
	bridge, err := d.extendedBridgeContract(ctx, baseURL, apiKey, chainIn)
	if err != nil {
		return "", nil, err
	}
	quote, err := d.extendedBridgeQuote(ctx, baseURL, apiKey, chainIn, chainOut, asset, plan.Amount)
	if err != nil {
		return "", nil, err
	}
	if plan.Account.Deposit.MaxFeeUSDC > 0 && quote.Fee > plan.Account.Deposit.MaxFeeUSDC {
		return "", nil, fmt.Errorf("extended bridge fee %.6f exceeds max_fee_usdc %.6f", quote.Fee, plan.Account.Deposit.MaxFeeUSDC)
	}
	if err := d.extendedCommitQuote(ctx, baseURL, apiKey, quote.ID); err != nil {
		return "", nil, err
	}
	commitmentID, err := parseCommitmentID(quote.ID)
	if err != nil {
		return "", nil, err
	}
	amount, err := usdcAmount(plan.Amount)
	if err != nil {
		return "", nil, err
	}
	txHash, approveHash, err := d.approveAndCall(ctx, wallet, common.HexToAddress(wallet.USDCAddress), common.HexToAddress(bridge), amount, extendedBridgeABI, "depositWithId", common.HexToAddress(wallet.USDCAddress), amount, commitmentID)
	metadata := map[string]any{
		"extended_bridge_contract": bridge,
		"extended_quote_id":        quote.ID,
		"extended_bridge_fee_usdc": quote.Fee,
		"approval_tx_hash":         approveHash,
	}
	return txHash, metadata, err
}

func (d EVMDepositor) asterTreasuryDeposit(ctx context.Context, plan DepositPlan) (string, map[string]any, error) {
	wallet := resolveWallet(plan)
	token := plan.Account.Deposit.TokenAddress
	if token == "" {
		return "", nil, fmt.Errorf("aster_treasury_deposit requires token_address")
	}
	treasury := strings.TrimSpace(plan.Account.Deposit.ToAddress)
	if treasury == "" {
		treasury = defaultAsterArbitrumTreasury
	}
	brokerID := plan.Account.Deposit.BrokerID
	if brokerID == 0 {
		brokerID = 1
	}
	amount, err := usdcAmount(plan.Amount)
	if err != nil {
		return "", nil, err
	}
	txHash, approveHash, err := d.approveAndCall(ctx, wallet, common.HexToAddress(token), common.HexToAddress(treasury), amount, asterTreasuryABI, "deposit", common.HexToAddress(token), amount, big.NewInt(brokerID))
	metadata := map[string]any{
		"aster_treasury":   treasury,
		"aster_broker_id":  brokerID,
		"token_address":    token,
		"approval_tx_hash": approveHash,
	}
	return txHash, metadata, err
}

func (d EVMDepositor) approveAndCall(ctx context.Context, wallet WalletConfig, token common.Address, spender common.Address, amount *big.Int, contractABI string, method string, args ...any) (string, string, error) {
	rpcURL := envValue(wallet.RPCURLEnv)
	if rpcURL == "" {
		return "", "", fmt.Errorf("missing %s", wallet.RPCURLEnv)
	}
	keyRaw := envValue(wallet.PrivateKeyEnv)
	if keyRaw == "" {
		return "", "", fmt.Errorf("missing %s", wallet.PrivateKeyEnv)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyRaw, "0x"))
	if err != nil {
		return "", "", fmt.Errorf("parse %s: %w", wallet.PrivateKeyEnv, err)
	}
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return "", "", err
	}
	defer client.Close()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return "", "", err
	}
	if wallet.ChainID > 0 && chainID.Int64() != wallet.ChainID {
		return "", "", fmt.Errorf("rpc chain id %s does not match configured chain id %d", chainID.String(), wallet.ChainID)
	}
	from := crypto.PubkeyToAddress(key.PublicKey)
	parsedERC20, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return "", "", err
	}
	balance, err := erc20Balance(ctx, client, token, from, parsedERC20)
	if err != nil {
		return "", "", fmt.Errorf("check token balance: %w", err)
	}
	if balance.Cmp(amount) < 0 {
		return "", "", fmt.Errorf("%s has insufficient token balance: need %s, have %s", wallet.PrivateKeyEnv, amount.String(), balance.String())
	}
	allowance, err := erc20Allowance(ctx, client, token, from, spender, parsedERC20)
	if err != nil {
		return "", "", fmt.Errorf("check token allowance: %w", err)
	}
	approveHash := ""
	if allowance.Cmp(amount) < 0 {
		data, err := parsedERC20.Pack("approve", spender, amount)
		if err != nil {
			return "", "", err
		}
		hash, err := d.sendContractTransaction(ctx, client, key, chainID, from, token, data)
		approveHash = hash
		if err != nil {
			return "", approveHash, err
		}
	}
	parsedContract, err := abi.JSON(strings.NewReader(contractABI))
	if err != nil {
		return "", approveHash, err
	}
	data, err := parsedContract.Pack(method, args...)
	if err != nil {
		return "", approveHash, err
	}
	txHash, err := d.sendContractTransaction(ctx, client, key, chainID, from, spender, data)
	return txHash, approveHash, err
}

func erc20Balance(ctx context.Context, client *ethclient.Client, token common.Address, owner common.Address, parsedABI abi.ABI) (*big.Int, error) {
	data, err := parsedABI.Pack("balanceOf", owner)
	if err != nil {
		return nil, err
	}
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	values, err := parsedABI.Unpack("balanceOf", raw)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("unexpected balanceOf return values")
	}
	balance, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected balanceOf type %T", values[0])
	}
	return balance, nil
}

func erc20Allowance(ctx context.Context, client *ethclient.Client, token common.Address, owner common.Address, spender common.Address, parsedABI abi.ABI) (*big.Int, error) {
	data, err := parsedABI.Pack("allowance", owner, spender)
	if err != nil {
		return nil, err
	}
	raw, err := client.CallContract(ctx, ethereum.CallMsg{To: &token, Data: data}, nil)
	if err != nil {
		return nil, err
	}
	values, err := parsedABI.Unpack("allowance", raw)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("unexpected allowance return values")
	}
	allowance, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unexpected allowance type %T", values[0])
	}
	return allowance, nil
}

func (d EVMDepositor) sendContractTransaction(ctx context.Context, client *ethclient.Client, key *ecdsa.PrivateKey, chainID *big.Int, from common.Address, to common.Address, data []byte) (string, error) {
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return "", err
	}
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Value: big.NewInt(0), Data: data})
	if err != nil {
		return "", fmt.Errorf("estimate gas: %w", err)
	}
	gasTip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		gasTip = big.NewInt(100_000_000)
	}
	paddedGasLimit := uint64(math.Ceil(float64(gasLimit) * 1.20))
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return "", err
	}
	tx, err := buildERC20TransferTx(ctx, client, header, chainID, nonce, to, data, gasTip, paddedGasLimit, from)
	if err != nil {
		return "", err
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key)
	if err != nil {
		return "", err
	}
	if err := client.SendTransaction(ctx, signed); err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, client, signed)
	if err != nil {
		return signed.Hash().Hex(), err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return signed.Hash().Hex(), fmt.Errorf("contract transaction reverted")
	}
	return signed.Hash().Hex(), nil
}

func buildERC20TransferTx(ctx context.Context, client *ethclient.Client, header *types.Header, chainID *big.Int, nonce uint64, token common.Address, data []byte, gasTip *big.Int, gasLimit uint64, from common.Address) (*types.Transaction, error) {
	if header.BaseFee == nil {
		gasPrice, err := client.SuggestGasPrice(ctx)
		if err != nil {
			return nil, err
		}
		if err := assertGasBalance(ctx, client, from, new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), gasPrice)); err != nil {
			return nil, err
		}
		return types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gasLimit,
			To:       &token,
			Value:    big.NewInt(0),
			Data:     data,
		}), nil
	}
	gasFeeCap := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	gasFeeCap.Add(gasFeeCap, gasTip)
	if err := assertGasBalance(ctx, client, from, new(big.Int).Mul(new(big.Int).SetUint64(gasLimit), gasFeeCap)); err != nil {
		return nil, err
	}
	return types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: gasTip,
		GasFeeCap: gasFeeCap,
		Gas:       gasLimit,
		To:        &token,
		Value:     big.NewInt(0),
		Data:      data,
	}), nil
}

func assertGasBalance(ctx context.Context, client *ethclient.Client, from common.Address, maxFee *big.Int) error {
	balance, err := client.BalanceAt(ctx, from, nil)
	if err != nil {
		return err
	}
	if balance.Cmp(maxFee) < 0 {
		return fmt.Errorf("%s has insufficient ETH for gas: need up to %s wei, have %s wei", from.Hex(), maxFee.String(), balance.String())
	}
	return nil
}

func (d EVMDepositor) lighterIntentAddress(ctx context.Context, plan DepositPlan) (string, map[string]any, error) {
	from, err := lighterCreditAddress(plan)
	if err != nil {
		return "", nil, err
	}
	baseURL := strings.TrimRight(plan.Account.Deposit.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://mainnet.zklighter.elliot.ai"
	}
	values := url.Values{}
	values.Set("chain_id", fmt.Sprint(resolveChainID(plan)))
	values.Set("from_addr", from)
	values.Set("amount", "0")
	values.Set("is_external_deposit", "true")
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/createIntentAddress", strings.NewReader(values.Encode()))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	var decoded any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := json.Marshal(decoded)
		return "", nil, fmt.Errorf("lighter createIntentAddress status %d: %s", resp.StatusCode, string(raw))
	}
	address := findAddress(decoded)
	if !common.IsHexAddress(address) {
		raw, _ := json.Marshal(decoded)
		return "", nil, fmt.Errorf("lighter createIntentAddress response did not contain an address: %s", string(raw))
	}
	return address, map[string]any{"lighter_from_address": from, "lighter_intent_response": decoded}, nil
}

type extendedBridgeQuote struct {
	ID  string
	Fee float64
}

func (d EVMDepositor) extendedBridgeContract(ctx context.Context, baseURL string, apiKey string, chain string) (string, error) {
	var decoded struct {
		Status string `json:"status"`
		Data   struct {
			Chains []struct {
				Chain           string `json:"chain"`
				ContractAddress string `json:"contractAddress"`
			} `json:"chains"`
		} `json:"data"`
		Error any `json:"error"`
	}
	if err := d.extendedJSON(ctx, http.MethodGet, baseURL+"/api/v1/user/bridge/config", apiKey, nil, &decoded); err != nil {
		return "", err
	}
	if strings.ToUpper(decoded.Status) != "OK" {
		return "", fmt.Errorf("extended bridge config returned status %q: %v", decoded.Status, decoded.Error)
	}
	for _, item := range decoded.Data.Chains {
		if strings.EqualFold(item.Chain, chain) {
			if !common.IsHexAddress(item.ContractAddress) {
				return "", fmt.Errorf("extended bridge contract for %s is invalid: %q", chain, item.ContractAddress)
			}
			return item.ContractAddress, nil
		}
	}
	return "", fmt.Errorf("extended bridge config does not include chain %q", chain)
}

func (d EVMDepositor) extendedBridgeQuote(ctx context.Context, baseURL string, apiKey string, chainIn string, chainOut string, asset string, amount float64) (extendedBridgeQuote, error) {
	values := url.Values{}
	values.Set("chainIn", chainIn)
	values.Set("chainOut", chainOut)
	values.Set("amount", fmt.Sprintf("%.6f", amount))
	values.Set("asset", asset)
	var decoded struct {
		Status string `json:"status"`
		Data   struct {
			ID  string `json:"id"`
			Fee string `json:"fee"`
		} `json:"data"`
		Error any `json:"error"`
	}
	if err := d.extendedJSON(ctx, http.MethodGet, baseURL+"/api/v1/user/bridge/quote?"+values.Encode(), apiKey, nil, &decoded); err != nil {
		return extendedBridgeQuote{}, err
	}
	if strings.ToUpper(decoded.Status) != "OK" {
		return extendedBridgeQuote{}, fmt.Errorf("extended bridge quote returned status %q: %v", decoded.Status, decoded.Error)
	}
	fee, err := parseFloat(decoded.Data.Fee)
	if err != nil {
		return extendedBridgeQuote{}, fmt.Errorf("parse extended bridge fee %q: %w", decoded.Data.Fee, err)
	}
	if decoded.Data.ID == "" {
		return extendedBridgeQuote{}, fmt.Errorf("extended bridge quote did not include id")
	}
	return extendedBridgeQuote{ID: decoded.Data.ID, Fee: fee}, nil
}

func (d EVMDepositor) extendedCommitQuote(ctx context.Context, baseURL string, apiKey string, quoteID string) error {
	values := url.Values{}
	values.Set("id", quoteID)
	var decoded struct {
		Status string `json:"status"`
		Error  any    `json:"error"`
	}
	if err := d.extendedJSON(ctx, http.MethodPost, baseURL+"/api/v1/user/bridge/quote?"+values.Encode(), apiKey, nil, &decoded); err != nil {
		return err
	}
	if strings.ToUpper(decoded.Status) != "OK" {
		return fmt.Errorf("extended bridge quote commit returned status %q: %v", decoded.Status, decoded.Error)
	}
	return nil
}

func (d EVMDepositor) extendedJSON(ctx context.Context, method string, endpoint string, apiKey string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("User-Agent", "perps-latency-benchmark")
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("extended request %s returned status %d", endpoint, resp.StatusCode)
	}
	return nil
}

func parseCommitmentID(value string) (*big.Int, error) {
	text := strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if text == "" {
		return nil, fmt.Errorf("empty quote id")
	}
	out := new(big.Int)
	if _, ok := out.SetString(text, 16); ok {
		return out, nil
	}
	if _, ok := out.SetString(text, 10); ok {
		return out, nil
	}
	return nil, fmt.Errorf("invalid quote id %q", value)
}

func parseFloat(value string) (float64, error) {
	parsed, _, err := big.ParseFloat(strings.TrimSpace(value), 10, 64, big.ToNearestEven)
	if err != nil {
		return 0, err
	}
	out, _ := parsed.Float64()
	return out, nil
}

func lighterCreditAddress(plan DepositPlan) (string, error) {
	if plan.Account.Deposit.FromAddressEnv != "" {
		from := envValue(plan.Account.Deposit.FromAddressEnv)
		if from == "" {
			return "", fmt.Errorf("missing %s", plan.Account.Deposit.FromAddressEnv)
		}
		if !common.IsHexAddress(from) {
			return "", fmt.Errorf("%s is not a valid address", plan.Account.Deposit.FromAddressEnv)
		}
		return common.HexToAddress(from).Hex(), nil
	}
	wallet := resolveWallet(plan)
	keyRaw := envValue(wallet.PrivateKeyEnv)
	if keyRaw == "" {
		return "", fmt.Errorf("missing %s", wallet.PrivateKeyEnv)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyRaw, "0x"))
	if err != nil {
		return "", err
	}
	return crypto.PubkeyToAddress(key.PublicKey).Hex(), nil
}

func resolveWallet(plan DepositPlan) WalletConfig {
	wallet := plan.Wallet
	if plan.Account.Deposit.ChainID != 0 {
		wallet.ChainID = plan.Account.Deposit.ChainID
	}
	if plan.Account.Deposit.RPCURLEnv != "" {
		wallet.RPCURLEnv = plan.Account.Deposit.RPCURLEnv
	}
	if plan.Account.Deposit.PrivateKeyEnv != "" {
		wallet.PrivateKeyEnv = plan.Account.Deposit.PrivateKeyEnv
	}
	if plan.Account.Deposit.USDCAddress != "" {
		wallet.USDCAddress = plan.Account.Deposit.USDCAddress
	}
	if wallet.ChainID == 0 {
		wallet.ChainID = DefaultArbitrumChainID
	}
	if wallet.RPCURLEnv == "" {
		wallet.RPCURLEnv = "ARBITRUM_RPC_URL"
	}
	if wallet.PrivateKeyEnv == "" {
		wallet.PrivateKeyEnv = "BENCHMARK_ARBITRUM_PRIVATE_KEY"
	}
	if wallet.USDCAddress == "" {
		wallet.USDCAddress = DefaultArbitrumNativeUSDC
	}
	return wallet
}

func resolveChainID(plan DepositPlan) int64 {
	chainID := plan.Account.Deposit.ChainID
	if chainID == 0 {
		chainID = plan.Wallet.ChainID
	}
	if chainID == 0 {
		chainID = DefaultArbitrumChainID
	}
	return chainID
}

func usdcAmount(amount float64) (*big.Int, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	text := fmt.Sprintf("%.6f", amount)
	parts := strings.SplitN(text, ".", 2)
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	for len(frac) < 6 {
		frac += "0"
	}
	value := new(big.Int)
	if _, ok := value.SetString(whole+frac[:6], 10); !ok {
		return nil, fmt.Errorf("invalid amount %q", text)
	}
	return value, nil
}

func findAddress(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"address", "intent_address", "intentAddress"} {
			if text, ok := typed[key].(string); ok && common.IsHexAddress(text) {
				return text
			}
		}
		for _, child := range typed {
			if address := findAddress(child); address != "" {
				return address
			}
		}
	case []any:
		for _, child := range typed {
			if address := findAddress(child); address != "" {
				return address
			}
		}
	}
	return ""
}

func envValue(name string) string {
	if name == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(name))
}

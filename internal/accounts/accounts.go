package accounts

import (
	"cmp"
	"crypto/ecdsa"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NethermindEth/starknet.go/curve"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"

	"perps-latency-benchmark/internal/names"
)

type WalletKind string

const (
	WalletEVM       WalletKind = "evm"
	WalletStark     WalletKind = "stark"
	WalletLighterL2 WalletKind = "lighter_l2"
)

type EnvVar struct {
	Name      string
	Wallet    WalletKind
	Secret    bool
	Generated bool
	Required  bool
	Note      string
}

type VenueSpec struct {
	Name        string
	WalletKinds []WalletKind
	Env         []EnvVar
	ManualSteps []string
	Supported   bool
}

type Wallet struct {
	Kind       WalletKind
	PrivateKey string
	PublicKey  string
	Address    string
}

type EnvStatus struct {
	Name     string
	Present  bool
	Required bool
	Secret   bool
	Note     string
}

type VenueStatus struct {
	Name        string
	WalletKinds []WalletKind
	Supported   bool
	Env         []EnvStatus
	ManualSteps []string
}

func Specs() []VenueSpec {
	out := append([]VenueSpec(nil), venueSpecs...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func Spec(name string) (VenueSpec, bool) {
	target := names.Normalize(name)
	for _, spec := range venueSpecs {
		if names.Normalize(spec.Name) == target {
			return spec, true
		}
	}
	return VenueSpec{}, false
}

func ResolveVenues(raw string) ([]VenueSpec, error) {
	if strings.TrimSpace(raw) == "" || strings.EqualFold(strings.TrimSpace(raw), "all") {
		return Specs(), nil
	}
	parts := strings.Split(raw, ",")
	specs := make([]VenueSpec, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		spec, ok := Spec(name)
		if !ok {
			return nil, fmt.Errorf("unknown account venue %q", name)
		}
		key := names.Normalize(spec.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		specs = append(specs, spec)
	}
	return specs, nil
}

func Status(specs []VenueSpec) []VenueStatus {
	statuses := make([]VenueStatus, 0, len(specs))
	for _, spec := range specs {
		status := VenueStatus{
			Name:        spec.Name,
			WalletKinds: WalletKinds(spec),
			Supported:   spec.Supported,
			ManualSteps: append([]string(nil), spec.ManualSteps...),
		}
		for _, env := range spec.Env {
			_, present := os.LookupEnv(env.Name)
			status.Env = append(status.Env, EnvStatus{
				Name:     env.Name,
				Present:  present,
				Required: env.Required,
				Secret:   env.Secret,
				Note:     env.Note,
			})
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func Check(specs []VenueSpec) error {
	var missing []string
	var unsupported []string
	for _, spec := range specs {
		if !spec.Supported {
			unsupported = append(unsupported, spec.Name)
			continue
		}
		for _, env := range spec.Env {
			if !env.Required {
				continue
			}
			if value, ok := os.LookupEnv(env.Name); !ok || value == "" {
				missing = append(missing, fmt.Sprintf("%s:%s", spec.Name, env.Name))
			}
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("venues are not ready for live account setup: %s", strings.Join(unsupported, ", "))
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required account environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func Generate(specs []VenueSpec, existing map[string]string) (map[string]string, map[WalletKind]Wallet, error) {
	values := cloneMap(existing)
	wallets := make(map[WalletKind]Wallet)
	for _, spec := range specs {
		if !spec.Supported {
			continue
		}
		for _, env := range spec.Env {
			if !env.Generated || values[env.Name] != "" {
				continue
			}
			kind := envWalletKind(spec, env)
			wallet, ok := wallets[kind]
			if !ok {
				var err error
				wallet, err = GenerateWallet(kind)
				if err != nil {
					return nil, nil, err
				}
				wallets[kind] = wallet
			}
			values[env.Name] = walletValue(wallet, env.Name)
		}
	}
	return values, wallets, nil
}

func GenerateWallet(kind WalletKind) (Wallet, error) {
	switch kind {
	case WalletEVM:
		key, err := ethcrypto.GenerateKey()
		if err != nil {
			return Wallet{}, err
		}
		return walletFromEVMKey(key), nil
	case WalletStark:
		privateKey, publicX, _, err := curve.GetRandomKeys()
		if err != nil {
			return Wallet{}, err
		}
		return Wallet{
			Kind:       WalletStark,
			PrivateKey: hexBig(privateKey),
			PublicKey:  hexBig(publicX),
		}, nil
	case WalletLighterL2:
		key := make([]byte, 40)
		if _, err := crand.Read(key); err != nil {
			return Wallet{}, err
		}
		return Wallet{
			Kind:       WalletLighterL2,
			PrivateKey: "0x" + hex.EncodeToString(key),
		}, nil
	default:
		return Wallet{}, fmt.Errorf("unsupported wallet kind %q", kind)
	}
}

func PublicFromEnv(kind WalletKind, env map[string]string) (Wallet, error) {
	switch kind {
	case WalletEVM:
		key := cmp.Or(env["HYPERLIQUID_SECRET_KEY"], env["GRVT_PRIVATE_KEY"], env["LIGHTER_L1_PRIVATE_KEY"])
		if key == "" {
			return Wallet{Kind: kind}, nil
		}
		privateKey, err := ethcrypto.HexToECDSA(strip0x(key))
		if err != nil {
			return Wallet{}, err
		}
		wallet := walletFromEVMKey(privateKey)
		wallet.PrivateKey = ""
		return wallet, nil
	case WalletStark:
		key := cmp.Or(env["EXTENDED_PRIVATE_KEY"], env["EDGEX_STARK_PRIVATE_KEY"])
		if key == "" {
			return Wallet{Kind: kind}, nil
		}
		privateKey, ok := new(big.Int).SetString(strip0x(key), 16)
		if !ok {
			return Wallet{}, fmt.Errorf("invalid stark private key")
		}
		publicX, _ := curve.PrivateKeyToPoint(privateKey)
		return Wallet{Kind: kind, PublicKey: hexBig(publicX)}, nil
	case WalletLighterL2:
		if env["LIGHTER_PRIVATE_KEY"] == "" {
			return Wallet{Kind: kind}, nil
		}
		return Wallet{Kind: kind}, nil
	default:
		return Wallet{}, fmt.Errorf("unsupported wallet kind %q", kind)
	}
}

func LoadDotenv(path string) (map[string]string, error) {
	if path == "" {
		return map[string]string{}, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return godotenv.Read(path)
}

func WriteDotenv(path string, values map[string]string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# Generated by perps-bench accounts generate.\n")
	b.WriteString("# Private keys are intentionally not printed by the CLI. Keep this file local.\n\n")
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(values[key])
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func EnvMap() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func RequiredWalletKinds(specs []VenueSpec) []WalletKind {
	seen := make(map[WalletKind]struct{})
	var kinds []WalletKind
	for _, spec := range specs {
		if !spec.Supported {
			continue
		}
		for _, kind := range WalletKinds(spec) {
			if _, exists := seen[kind]; exists {
				continue
			}
			seen[kind] = struct{}{}
			kinds = append(kinds, kind)
		}
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	return kinds
}

func WalletKinds(spec VenueSpec) []WalletKind {
	seen := make(map[WalletKind]struct{})
	var kinds []WalletKind
	add := func(kind WalletKind) {
		if kind == "" {
			return
		}
		if _, exists := seen[kind]; exists {
			return
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	for _, kind := range spec.WalletKinds {
		add(kind)
	}
	for _, env := range spec.Env {
		if env.Generated {
			add(envWalletKind(spec, env))
		}
	}
	sort.Slice(kinds, func(i, j int) bool {
		return kinds[i] < kinds[j]
	})
	return kinds
}

func walletFromEVMKey(key *ecdsa.PrivateKey) Wallet {
	return Wallet{
		Kind:       WalletEVM,
		PrivateKey: "0x" + hex.EncodeToString(ethcrypto.FromECDSA(key)),
		Address:    ethcrypto.PubkeyToAddress(key.PublicKey).Hex(),
	}
}

func walletValue(wallet Wallet, envName string) string {
	switch envName {
	case "EXTENDED_PUBLIC_KEY":
		return wallet.PublicKey
	case "LIGHTER_L1_ADDRESS":
		return wallet.Address
	default:
		return wallet.PrivateKey
	}
}

func envWalletKind(spec VenueSpec, env EnvVar) WalletKind {
	if env.Wallet != "" {
		return env.Wallet
	}
	for _, kind := range spec.WalletKinds {
		if kind != "" {
			return kind
		}
	}
	return ""
}

func hexBig(value *big.Int) string {
	return "0x" + fmt.Sprintf("%064x", value)
}

func strip0x(value string) string {
	return strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

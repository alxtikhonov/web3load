package action

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// coerceArgs converts YAML-decoded values (string/int/float64/bool/
// []interface{}) into the Go types go-ethereum's abi.Pack expects, guided
// by each method input's declared ABI type. It covers the scalar and
// single-level array/slice cases real scenarios need (docs/dsl-reference.md);
// nested tuples/structs are a known v0.1 gap.
func coerceArgs(inputs abi.Arguments, raw []interface{}) ([]interface{}, error) {
	if len(inputs) != len(raw) {
		return nil, fmt.Errorf("expected %d args, got %d", len(inputs), len(raw))
	}
	out := make([]interface{}, len(raw))
	for i, arg := range inputs {
		v, err := coerceOne(arg.Type, raw[i])
		if err != nil {
			return nil, fmt.Errorf("arg %d (%s): %w", i, arg.Name, err)
		}
		out[i] = v
	}
	return out, nil
}

func coerceOne(t abi.Type, raw interface{}) (interface{}, error) {
	switch t.T {
	case abi.AddressTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected address string, got %T", raw)
		}
		return common.HexToAddress(s), nil

	case abi.UintTy, abi.IntTy:
		return toBigInt(raw)

	case abi.BoolTy:
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool, got %T", raw)
		}
		return b, nil

	case abi.StringTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", raw)
		}
		return s, nil

	case abi.BytesTy, abi.FixedBytesTy:
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("expected 0x-hex string for bytes, got %T", raw)
		}
		return hexToBytes(s)

	case abi.SliceTy, abi.ArrayTy:
		items, ok := raw.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected list, got %T", raw)
		}
		elemGoType := t.Elem.GetType()
		result := reflect.MakeSlice(reflect.SliceOf(elemGoType), len(items), len(items))
		for i, it := range items {
			v, err := coerceOne(*t.Elem, it)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			result.Index(i).Set(reflect.ValueOf(v))
		}
		return result.Interface(), nil

	default:
		return nil, fmt.Errorf("abi type %s is not supported by contract_call in v0.1", t.String())
	}
}

func toBigInt(raw interface{}) (*big.Int, error) {
	switch v := raw.(type) {
	case int:
		return big.NewInt(int64(v)), nil
	case int64:
		return big.NewInt(v), nil
	case uint64:
		return new(big.Int).SetUint64(v), nil
	case float64:
		return big.NewInt(int64(v)), nil
	case string:
		n, ok := new(big.Int).SetString(v, 10)
		if !ok {
			return nil, fmt.Errorf("invalid integer string %q", v)
		}
		return n, nil
	default:
		return nil, fmt.Errorf("expected integer, got %T", raw)
	}
}

func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex %q: %w", s, err)
	}
	return b, nil
}

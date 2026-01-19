package types

import "errors"

var (
	ErrInvalidEthAddress = errors.New("invalid Ethereum address format")
	ErrInvalidKeccak256  = errors.New("invalid Keccak256 hash format")
	ErrInvalidHexString  = errors.New("invalid hex string format")
)

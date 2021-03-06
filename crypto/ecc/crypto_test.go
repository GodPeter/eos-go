package ecc

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateToPublic(t *testing.T) {
	wif := "5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjss"
	privKey, err := NewPrivateKey(wif)
	require.NoError(t, err)

	pubKey := privKey.PublicKey()

	pubKeyString := pubKey.String()
	assert.Equal(t, "EOS859gxfnXyUriMgUeThh1fWv3oqcpLFyHa3TfFYC4PK2HqhToVM", pubKeyString)

}

func TestNewPublicKeyAndSerializeCompress(t *testing.T) {
	// Copied test from eosjs(-.*)?
	key, err := NewPublicKey("EOS6MRyAjQq8ud7hVNYcfnVPJqcVpscN5So8BhtHuGYqET5GDW5CV")
	require.NoError(t, err)
	assert.Equal(t, "02c0ded2bc1f1305fb0faac5e6c03ee3a1924234985427b6167ca569d13df435cf", hex.EncodeToString(key.Content[:]))
}

func TestNewRandomPrivateKey(t *testing.T) {
	key, err := NewRandomPrivateKey()
	require.NoError(t, err)
	// taken from eosiojs-ecc:common.test.js:12
	assert.Regexp(t, "^5[HJK].*", key.String())
}

func TestPrivateKeyValidity(t *testing.T) {
	tests := []struct {
		in    string
		valid bool
	}{
		{"5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjss", true},
		{"5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjsm", false},
	}

	for _, test := range tests {
		_, err := NewPrivateKey(test.in)
		if test.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
			assert.Equal(t, "checksum mismatch", err.Error())
		}
	}
}

func TestPublicKeyValidity(t *testing.T) {
	tests := []struct {
		in  string
		err error
	}{
		{"EOS859gxfnXyUriMgUeThh1fWv3oqcpLFyHa3TfFYC4PK2HqhToVM", nil},
		{"MMM859gxfnXyUriMgUeThh1fWv3oqcpLFyHa3TfFYC4PK2HqhToVM", fmt.Errorf("public key should start with \"PUB_\" (or the old \"EOS\")")},
		{"EOS859gxfnXyUriMgUeThh1fWv3oqcpLFyHa3TfFYC4PK2HqhTo", fmt.Errorf("checkDecode: invalid checksum")},
	}

	for idx, test := range tests {
		_, err := NewPublicKey(test.in)
		if test.err == nil {
			assert.NoError(t, err, fmt.Sprintf("test %d with key %q", idx, test.in))
		} else {
			assert.Error(t, err)
			assert.Equal(t, test.err.Error(), err.Error())
		}
	}
}

func TestSignature(t *testing.T) {
	wif := "5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjss"
	privKey, err := NewPrivateKey(wif)
	require.NoError(t, err)

	cnt := []byte("hi")
	digest := sigDigest([]byte{}, cnt)
	signature, err := privKey.Sign(digest)
	require.NoError(t, err)

	assert.True(t, signature.Verify(digest, privKey.PublicKey()))
}

func TestPubCompare(t *testing.T) {
	wif1 := "5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjss"
	privKey1, err := NewPrivateKey(wif1)
	require.NoError(t, err)

	wif2 := "5KYZdUEo39z3FPrtuX2QbbwGnNP5zTd7yyr2SC1j299sBCnWjss"
	privKey2, err := NewPrivateKey(wif2)
	require.NoError(t, err)
	assert.True(t, privKey1.PublicKey().Compare(privKey2.PublicKey()))

	wif3 := "5JGFVCESmDHEK4HWY5CEYxh5xQfg8vJpUVhcEUJLXsHPXfPhkDU"
	privKey3, err := NewPrivateKey(wif3)
	require.NoError(t, err)
	assert.Equal(t, false, privKey1.PublicKey().Compare(privKey3.PublicKey()))

}

func TestPublicValid(t *testing.T) {
	trueContent := [33]uint8{0x3, 0xaf, 0xc2, 0xef, 0x7e, 0x9f, 0xae, 0x43, 0x24, 0xed, 0xeb, 0xf1, 0xd5, 0x25, 0x44, 0x59, 0xff, 0x17, 0xf6, 0xf3, 0xf1, 0x3e, 0x3d, 0xa2, 0x20, 0x53, 0xcf, 0x9c, 0xc3, 0xfc, 0x46, 0x8b, 0xf8}
	key1 := PublicKey{Curve: 0, Content: trueContent}
	assert.Equal(t, true, key1.Valid())

	falseContent := [33]uint8{0x3, 0xaf, 0xc2, 0xef, 0x7e, 0x39, 0xae, 0x43, 0x24, 0xed, 0xeb, 0xf1, 0xd5, 0x25, 0x44, 0x59, 0xff, 0x17, 0xf6, 0xf3, 0xf1, 0x3e, 0x3d, 0xa2, 0x20, 0x53, 0xcf, 0x9c, 0xc3, 0xfc, 0x46, 0x8b, 0xf8}
	key2 := PublicKey{Curve: 0, Content: falseContent}
	assert.Equal(t, false, key2.Valid())
}

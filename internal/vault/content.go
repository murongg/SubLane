package vault

import "fmt"

func (v *Vault) SealContent(tenantID int64, plaintext []byte) ([]byte, error) {
	if tenantID <= 0 {
		return nil, ErrDecrypt
	}
	return v.seal(fmt.Sprintf("sublane:content:%d", tenantID), plaintext)
}

func (v *Vault) OpenContent(tenantID int64, encrypted []byte) ([]byte, error) {
	if tenantID <= 0 {
		return nil, ErrDecrypt
	}
	return v.open(fmt.Sprintf("sublane:content:%d", tenantID), encrypted)
}

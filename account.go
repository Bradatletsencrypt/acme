package acme

import (
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

// NewAccount registers a new account with the acme service
func (c Client) NewAccount(privateKey crypto.Signer, onlyReturnExisting, termsOfServiceAgreed bool, contact ...string) (Account, error) {
	newAccountReq := struct {
		OnlyReturnExisting   bool     `json:"onlyReturnExisting"`
		TermsOfServiceAgreed bool     `json:"termsOfServiceAgreed"`
		Contact              []string `json:"contact,omitempty"`
	}{
		OnlyReturnExisting:   onlyReturnExisting,
		TermsOfServiceAgreed: termsOfServiceAgreed,
		Contact:              contact,
	}

	account := Account{}
	resp, err := c.post(c.dir.NewAccount, "", privateKey, newAccountReq, &account, http.StatusOK, http.StatusCreated)
	if err != nil {
		return account, err
	}

	account.URL = resp.Header.Get("Location")
	account.PrivateKey = privateKey

	if account.Thumbprint == "" {
		account.Thumbprint, err = JWKThumbprint(account.PrivateKey.Public())
		if err != nil {
			return account, fmt.Errorf("acme: error computing account thumbprint: %v", err)
		}
	}

	return account, nil
}

// NewAccountExternalBinding registers an account with an acme server that requires external account binding
func (c Client) NewAccountExternalBinding(privateKey crypto.Signer, onlyReturnExisting, termsOfServiceAgreed bool,
	eab ExternalAccountBinding, contact ...string) (Account, error) {
	newAccountReq := struct {
		OnlyReturnExisting     bool            `json:"onlyReturnExisting"`
		TermsOfServiceAgreed   bool            `json:"termsOfServiceAgreed"`
		Contact                []string        `json:"contact,omitempty"`
		ExternalAccountBinding json.RawMessage `json:"externalAccountBinding"`
	}{
		OnlyReturnExisting:   onlyReturnExisting,
		TermsOfServiceAgreed: termsOfServiceAgreed,
		Contact:              contact,
	}

	account := Account{}

	jwsEab, err := jwsEncodeEAB(privateKey, keyID(eab.KeyIdentifier), eab.MacKey, c.dir.NewAccount, eab.HashFunc)
	if err != nil {
		return account, fmt.Errorf("acme: error computing external account binding jws: %v", err)
	}
	newAccountReq.ExternalAccountBinding = jwsEab

	resp, err := c.post(c.dir.NewAccount, "", privateKey, newAccountReq, &account, http.StatusOK, http.StatusCreated)
	if err != nil {
		return account, err
	}

	account.URL = resp.Header.Get("Location")
	account.PrivateKey = privateKey

	if account.Thumbprint == "" {
		account.Thumbprint, err = JWKThumbprint(account.PrivateKey.Public())
		if err != nil {
			return account, fmt.Errorf("acme: error computing account thumbprint: %v", err)
		}
	}

	account.ExternalAccountBinding = eab

	return account, nil
}

// UpdateAccount updates an existing account with the acme service.
func (c Client) UpdateAccount(account Account, contact ...string) (Account, error) {
	var updateAccountReq interface{}

	if !reflect.DeepEqual(account.Contact, contact) {
		// Only provide a non-nil updateAccountReq when there is an update to be made.
		updateAccountReq = struct {
			Contact []string `json:"contact,omitempty"`
		}{
			Contact: contact,
		}
	} else {
		// Otherwise use "" to trigger a POST-as-GET to fetch up-to-date account
		// information from the acme service.
		updateAccountReq = ""
	}

	_, err := c.post(account.URL, account.URL, account.PrivateKey, updateAccountReq, &account, http.StatusOK)
	if err != nil {
		return account, err
	}

	if account.Thumbprint == "" {
		account.Thumbprint, err = JWKThumbprint(account.PrivateKey.Public())
		if err != nil {
			return account, fmt.Errorf("acme: error computing account thumbprint: %v", err)
		}
	}

	return account, nil
}

// AccountKeyChange rolls over an account to a new key.
func (c Client) AccountKeyChange(account Account, newPrivateKey crypto.Signer) (Account, error) {
	oldJwkKeyPub, err := jwkEncode(account.PrivateKey.Public())
	if err != nil {
		return account, fmt.Errorf("acme: error encoding new private key: %v", err)
	}

	keyChangeReq := struct {
		Account string          `json:"account"`
		OldKey  json.RawMessage `json:"oldKey"`
	}{
		Account: account.URL,
		OldKey:  []byte(oldJwkKeyPub),
	}

	innerJws, err := jwsEncodeJSON(keyChangeReq, newPrivateKey, "", "", c.dir.KeyChange)
	if err != nil {
		return account, fmt.Errorf("acme: error encoding inner jws: %v", err)
	}

	if _, err := c.post(c.dir.KeyChange, account.URL, account.PrivateKey, json.RawMessage(innerJws), nil, http.StatusOK); err != nil {
		return account, err
	}

	account.PrivateKey = newPrivateKey

	return account, nil
}

// DeactivateAccount deactivates a given account.
func (c Client) DeactivateAccount(account Account) (Account, error) {
	deactivateReq := struct {
		Status string `json:"status"`
	}{
		Status: "deactivated",
	}

	_, err := c.post(account.URL, account.URL, account.PrivateKey, deactivateReq, &account, http.StatusOK)

	return account, err
}

// FetchOrderList fetches a list of orders from the account url provided in the account Orders field
func (c Client) FetchOrderList(account Account) (OrderList, error) {
	orderList := OrderList{}

	if account.Orders == "" {
		return orderList, errors.New("no order list for account")
	}

	_, err := c.post(account.Orders, account.URL, account.PrivateKey, "", &orderList, http.StatusOK)

	return orderList, err
}

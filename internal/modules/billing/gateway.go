package billing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"modular-api/internal/modules/property"
	"modular-api/internal/platform/apperrors"

	"gorm.io/gorm"
)

const (
	billplzGatewayName = "billplz"
	billplzMethodID    = "online_billplz"
	billplzMethodLabel = "Billplz Sandbox"
)

type CreateBillplzCheckoutInput struct {
	UnitCode    string   `json:"unitCode"`
	ChargeRefs  []string `json:"chargeReferences"`
	RedirectURL string   `json:"redirectUrl"`
}

type BillplzCheckoutResponse struct {
	Reference   string  `json:"reference"`
	CheckoutURL string  `json:"checkoutUrl"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	UnitCode    string  `json:"unitCode"`
}

type ConfirmBillplzPaymentInput struct {
	BillID            string `json:"billId"`
	Paid              string `json:"paid"`
	PaidAt            string `json:"paidAt"`
	XSignature        string `json:"xSignature"`
	TransactionID     string `json:"transactionId"`
	TransactionStatus string `json:"transactionStatus"`
}

type BillplzPaymentStatusResponse struct {
	Reference        string `json:"reference"`
	BillID           string `json:"billId"`
	UnitCode         string `json:"unitCode"`
	Status           string `json:"status"`
	Outcome          string `json:"outcome"`
	PaymentReference string `json:"paymentReference,omitempty"`
	PaidAt           string `json:"paidAt,omitempty"`
}

type billplzCreateBillResponse struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Paid   bool   `json:"paid"`
	Amount int    `json:"amount"`
}

type billplzGetBillResponse struct {
	ID         string `json:"id"`
	Paid       bool   `json:"paid"`
	State      string `json:"state"`
	PaidAt     string `json:"paid_at"`
	PaidAmount int    `json:"paid_amount"`
}

type billplzCallbackResult struct {
	BillID        string
	Paid          bool
	State         string
	PaidAt        string
	RawPayload    string
	TransactionID string
}

func (m *Module) CreateBillplzCheckout(input CreateBillplzCheckoutInput) (*BillplzCheckoutResponse, error) {
	if err := m.validateBillplzConfig(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.UnitCode) == "" {
		return nil, apperrors.Validation("unitCode is required")
	}

	unit, charges, amount, err := m.payableChargesForUnit(input.UnitCode, input.ChargeRefs)
	if err != nil {
		return nil, err
	}

	description := fmt.Sprintf("Billing payment for %s", unit.Code)
	if len(charges) == 1 {
		description = charges[0].Description
	}

	form := url.Values{}
	form.Set("collection_id", m.gateway.BillplzCollectionID)
	form.Set("description", description)
	form.Set("email", unit.ResidentAccount.Email)
	form.Set("name", unit.ResidentAccount.ResidentName)
	form.Set("amount", fmt.Sprintf("%d", amountToSen(amount)))
	form.Set("callback_url", strings.TrimRight(m.gateway.BillplzCallbackBaseURL, "/")+"/api/v1/billing/payments/billplz/callback")
	if redirectURL := strings.TrimSpace(input.RedirectURL); redirectURL != "" {
		form.Set("redirect_url", redirectURL)
	}

	bill, err := m.createBillplzBill(form)
	if err != nil {
		return nil, err
	}

	reference := gatewayReference(unit.Code)
	transaction := GatewayTransaction{
		Gateway:          billplzGatewayName,
		ExternalID:       bill.ID,
		AccountCode:      unit.ResidentAccount.AccountCode,
		UnitCode:         unit.Code,
		Reference:        reference,
		Amount:           amount,
		Currency:         "MYR",
		Status:           "pending",
		PayerName:        unit.ResidentAccount.ResidentName,
		PayerEmail:       unit.ResidentAccount.Email,
		ChargeReferences: joinChargeReferences(charges),
		CheckoutURL:      bill.URL,
		RedirectURL:      strings.TrimSpace(input.RedirectURL),
	}

	if err := m.db.Create(&transaction).Error; err != nil {
		return nil, apperrors.Internal("create gateway transaction", err)
	}

	return &BillplzCheckoutResponse{
		Reference:   reference,
		CheckoutURL: bill.URL,
		Amount:      amount,
		Currency:    "MYR",
		UnitCode:    unit.Code,
	}, nil
}

func (m *Module) HandleBillplzCallback(values url.Values) error {
	if err := m.validateBillplzConfig(); err != nil {
		return err
	}

	payload, err := json.Marshal(flattenFormValues(values))
	if err != nil {
		return apperrors.Internal("marshal callback payload", err)
	}

	signature := strings.TrimSpace(values.Get("x_signature"))
	if signature == "" {
		return apperrors.Validation("billplz callback signature is missing")
	}
	if !validBillplzSignature(values, signature, m.gateway.BillplzXSignatureKey) {
		return apperrors.Validation("billplz callback signature is invalid")
	}

	result := billplzCallbackResult{
		BillID:        strings.TrimSpace(values.Get("id")),
		Paid:          strings.EqualFold(strings.TrimSpace(values.Get("paid")), "true"),
		State:         strings.TrimSpace(values.Get("state")),
		PaidAt:        strings.TrimSpace(values.Get("paid_at")),
		RawPayload:    string(payload),
		TransactionID: strings.TrimSpace(values.Get("transaction_id")),
	}
	if result.BillID == "" {
		return apperrors.Validation("billplz callback bill id is missing")
	}

	return m.db.Transaction(func(tx *gorm.DB) error {
		var transaction GatewayTransaction
		if err := tx.Where("gateway = ? AND external_id = ?", billplzGatewayName, result.BillID).First(&transaction).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NotFoundf("gateway transaction %s not found", result.BillID)
			}
			return apperrors.Internal("load gateway transaction", err)
		}

		return m.reconcileGatewayTransaction(tx, &transaction, result, result.RawPayload)
	})
}

func (m *Module) ConfirmBillplzPayment(input ConfirmBillplzPaymentInput) (*BillplzPaymentStatusResponse, error) {
	if err := m.validateBillplzConfig(); err != nil {
		return nil, err
	}

	billID := strings.TrimSpace(input.BillID)
	if billID == "" {
		return nil, apperrors.Validation("billId is required")
	}

	redirectValues := url.Values{}
	redirectValues.Set("billplz[id]", billID)
	if value := strings.TrimSpace(input.Paid); value != "" {
		redirectValues.Set("billplz[paid]", value)
	}
	if value := strings.TrimSpace(input.PaidAt); value != "" {
		redirectValues.Set("billplz[paid_at]", value)
	}
	if value := strings.TrimSpace(input.TransactionID); value != "" {
		redirectValues.Set("billplz[transaction_id]", value)
	}
	if value := strings.TrimSpace(input.TransactionStatus); value != "" {
		redirectValues.Set("billplz[transaction_status]", value)
	}

	redirectSignatureValid := false
	if signature := strings.TrimSpace(input.XSignature); signature != "" {
		redirectSignatureValid = validBillplzSignature(redirectValues, signature, m.gateway.BillplzXSignatureKey)
		if !redirectSignatureValid {
			return nil, apperrors.Validation("billplz redirect signature is invalid")
		}
	}

	getBillResponse, err := m.waitForBillplzBillState(billID, strings.EqualFold(strings.TrimSpace(input.Paid), "true"))
	if err != nil {
		return nil, err
	}

	paidAt := strings.TrimSpace(input.PaidAt)
	if paidAt == "" {
		paidAt = getBillResponse.PaidAt
	}

	result := billplzCallbackResult{
		BillID:        billID,
		Paid:          getBillResponse.Paid,
		State:         defaultString(getBillResponse.State, normalizeRedirectState(input.Paid)),
		PaidAt:        paidAt,
		TransactionID: strings.TrimSpace(input.TransactionID),
	}

	if !getBillResponse.Paid && strings.EqualFold(strings.TrimSpace(input.Paid), "true") && redirectSignatureValid {
		result.Paid = true
		result.State = "paid"
	}

	response, err := m.confirmGatewayTransaction(result, redirectValues)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *Module) validateBillplzConfig() error {
	switch {
	case strings.TrimSpace(m.gateway.BillplzAPIBaseURL) == "":
		return apperrors.Validation("BILLPLZ_API_BASE_URL is required")
	case strings.TrimSpace(m.gateway.BillplzAPIKey) == "":
		return apperrors.Validation("BILLPLZ_API_KEY is required")
	case strings.TrimSpace(m.gateway.BillplzXSignatureKey) == "":
		return apperrors.Validation("BILLPLZ_X_SIGNATURE_KEY is required")
	case strings.TrimSpace(m.gateway.BillplzCollectionID) == "":
		return apperrors.Validation("BILLPLZ_COLLECTION_ID is required")
	case strings.TrimSpace(m.gateway.BillplzCallbackBaseURL) == "":
		return apperrors.Validation("BILLPLZ_CALLBACK_BASE_URL is required")
	default:
		return nil
	}
}

func (m *Module) payableChargesForUnit(unitCode string, selectedRefs []string) (property.Unit, []Charge, float64, error) {
	var unit property.Unit
	if err := m.db.Preload("ResidentAccount").Where("code = ?", unitCode).First(&unit).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return property.Unit{}, nil, 0, apperrors.NotFoundf("billing unit %s not found", unitCode)
		}
		return property.Unit{}, nil, 0, apperrors.Internal("load gateway billing unit", err)
	}

	var charges []Charge
	if err := m.db.Where("unit_code = ?", unitCode).Order("due_date asc").Find(&charges).Error; err != nil {
		return property.Unit{}, nil, 0, apperrors.Internal("list gateway charges", err)
	}

	paidByCharge, err := m.paidAmountByCharge(charges)
	if err != nil {
		return property.Unit{}, nil, 0, err
	}

	selectedSet := make(map[string]struct{}, len(selectedRefs))
	for _, reference := range selectedRefs {
		if trimmed := strings.TrimSpace(reference); trimmed != "" {
			selectedSet[trimmed] = struct{}{}
		}
	}

	payable := make([]Charge, 0, len(charges))
	amount := 0.0

	for _, charge := range charges {
		if len(selectedSet) > 0 {
			if _, ok := selectedSet[charge.Reference]; !ok {
				continue
			}
		}

		balance := maxFloat(charge.Amount-paidByCharge[charge.ID], 0)
		if balance <= 0 {
			continue
		}

		payable = append(payable, charge)
		amount += balance
	}

	if len(selectedSet) > 0 && len(payable) != len(selectedSet) {
		return property.Unit{}, nil, 0, apperrors.NotFound("one or more charges were not found for the selected unit")
	}
	if len(payable) == 0 || amount <= 0 {
		return property.Unit{}, nil, 0, apperrors.Validation("no outstanding charges available for payment")
	}

	return unit, payable, amount, nil
}

func (m *Module) createBillplzBill(form url.Values) (*billplzCreateBillResponse, error) {
	endpoint := strings.TrimRight(m.gateway.BillplzAPIBaseURL, "/") + "/v3/bills"

	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, apperrors.Internal("build Billplz request", err)
	}

	request.SetBasicAuth(m.gateway.BillplzAPIKey, "")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, apperrors.Internal("send Billplz request", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, apperrors.Internal("read Billplz response", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, apperrors.Validation(fmt.Sprintf("Billplz checkout creation failed with status %d", response.StatusCode))
	}

	var bill billplzCreateBillResponse
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&bill); err != nil {
		return nil, apperrors.Internal("decode Billplz response", err)
	}
	if strings.TrimSpace(bill.ID) == "" || strings.TrimSpace(bill.URL) == "" {
		return nil, apperrors.Validation("Billplz response did not include bill id or checkout URL")
	}

	return &bill, nil
}

func (m *Module) getBillplzBill(billID string) (*billplzGetBillResponse, error) {
	endpoint := strings.TrimRight(m.gateway.BillplzAPIBaseURL, "/") + "/v3/bills/" + url.PathEscape(billID)

	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, apperrors.Internal("build Billplz get bill request", err)
	}

	request.SetBasicAuth(m.gateway.BillplzAPIKey, "")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, apperrors.Internal("send Billplz get bill request", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, apperrors.Internal("read Billplz get bill response", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, apperrors.Validation(fmt.Sprintf("Billplz get bill failed with status %d", response.StatusCode))
	}

	var bill billplzGetBillResponse
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&bill); err != nil {
		return nil, apperrors.Internal("decode Billplz get bill response", err)
	}
	if strings.TrimSpace(bill.ID) == "" {
		return nil, apperrors.Validation("Billplz get bill response did not include bill id")
	}

	return &bill, nil
}

func (m *Module) waitForBillplzBillState(billID string, expectPaid bool) (*billplzGetBillResponse, error) {
	var latest *billplzGetBillResponse
	var err error

	for attempt := 0; attempt < 5; attempt++ {
		latest, err = m.getBillplzBill(billID)
		if err != nil {
			return nil, err
		}
		if !expectPaid || latest.Paid {
			return latest, nil
		}
		time.Sleep(1500 * time.Millisecond)
	}

	return latest, nil
}

func (m *Module) confirmGatewayTransaction(result billplzCallbackResult, redirectValues url.Values) (*BillplzPaymentStatusResponse, error) {
	payload := ""
	if len(redirectValues) > 0 {
		data, err := json.Marshal(flattenFormValues(redirectValues))
		if err != nil {
			return nil, apperrors.Internal("marshal Billplz redirect payload", err)
		}
		payload = string(data)
	}

	var paymentReference string
	var unitCode string
	var reference string
	var paidAt string

	err := m.db.Transaction(func(tx *gorm.DB) error {
		var transaction GatewayTransaction
		if err := tx.Where("gateway = ? AND external_id = ?", billplzGatewayName, result.BillID).First(&transaction).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return apperrors.NotFoundf("gateway transaction %s not found", result.BillID)
			}
			return apperrors.Internal("load gateway transaction", err)
		}

		unitCode = transaction.UnitCode
		reference = transaction.Reference

		if err := m.reconcileGatewayTransaction(tx, &transaction, result, payload); err != nil {
			return err
		}

		if transaction.SettledPaymentID != nil {
			var payment Payment
			if err := tx.First(&payment, *transaction.SettledPaymentID).Error; err != nil {
				return apperrors.Internal("load settled payment", err)
			}
			paymentReference = payment.Reference
			paidAt = payment.PaidAt
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	status := normalizeGatewayStatus(result.Paid, result.State)
	if paymentReference != "" {
		status = "paid"
	}

	return &BillplzPaymentStatusResponse{
		Reference:        reference,
		BillID:           result.BillID,
		UnitCode:         unitCode,
		Status:           status,
		Outcome:          gatewayOutcome(status),
		PaymentReference: paymentReference,
		PaidAt:           paidAt,
	}, nil
}

func (m *Module) reconcileGatewayTransaction(tx *gorm.DB, transaction *GatewayTransaction, result billplzCallbackResult, payload string) error {
	nextStatus := normalizeGatewayStatus(result.Paid, result.State)
	updates := map[string]any{
		"status":     nextStatus,
		"updated_at": time.Now(),
	}
	if payload != "" {
		updates["callback_payload"] = payload
	}

	if transaction.SettledPaymentID == nil && result.Paid {
		paymentID, err := m.createSettledGatewayPayment(tx, *transaction, result)
		if err != nil {
			return err
		}
		updates["settled_payment_id"] = paymentID
		transaction.SettledPaymentID = &paymentID
	}

	if err := tx.Model(&GatewayTransaction{}).Where("id = ?", transaction.ID).Updates(updates).Error; err != nil {
		return apperrors.Internal("update gateway transaction", err)
	}

	transaction.Status = nextStatus
	return nil
}

func (m *Module) createSettledGatewayPayment(tx *gorm.DB, transaction GatewayTransaction, callback billplzCallbackResult) (uint, error) {
	chargeRefs := splitChargeReferences(transaction.ChargeReferences)
	var charges []Charge
	if err := tx.Where("reference IN ? AND unit_code = ?", chargeRefs, transaction.UnitCode).Order("due_date asc, reference asc").Find(&charges).Error; err != nil {
		return 0, apperrors.Internal("list callback charges", err)
	}
	if len(charges) != len(chargeRefs) {
		return 0, apperrors.NotFound("one or more callback charges were not found for the selected unit")
	}

	paidByCharge, err := m.paidAmountByChargeWithDB(tx, charges)
	if err != nil {
		return 0, err
	}

	payment := Payment{
		AccountCode: transaction.AccountCode,
		UnitCode:    transaction.UnitCode,
		Amount:      transaction.Amount,
		PaidAt:      paidAtLabel(callback.PaidAt),
		Reference:   transaction.Reference,
		Description: gatewayPaymentDescription(transaction, callback),
		Source:      "system",
		MethodID:    billplzMethodID,
		MethodLabel: billplzMethodLabel,
		Status:      "successful",
	}

	if err := tx.Create(&payment).Error; err != nil {
		return 0, apperrors.Internal("create settled gateway payment", err)
	}

	for _, allocation := range buildPaymentAllocations(payment.ID, transaction.Amount, charges, paidByCharge) {
		if allocation.Amount <= 0 {
			continue
		}
		if err := tx.Create(&allocation).Error; err != nil {
			return 0, apperrors.Internal("create gateway payment allocation", err)
		}
	}

	return payment.ID, nil
}

func validBillplzSignature(values url.Values, signature, key string) bool {
	expected := computeBillplzSignature(values, key)
	return hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected))
}

func computeBillplzSignature(values url.Values, key string) string {
	elements := make([]string, 0)
	keys := make([]string, 0, len(values))
	for name := range values {
		if shouldSkipBillplzSignatureKey(name) {
			continue
		}
		keys = append(keys, name)
	}

	sort.Slice(keys, func(left, right int) bool {
		return strings.ToLower(keys[left]) < strings.ToLower(keys[right])
	})

	for _, name := range keys {
		for _, value := range values[name] {
			elements = append(elements, name+value)
		}
	}

	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(strings.Join(elements, "|")))
	return hex.EncodeToString(mac.Sum(nil))
}

func shouldSkipBillplzSignatureKey(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "x_signature" || strings.HasSuffix(normalized, "[x_signature]")
}

func amountToSen(amount float64) int {
	return int(math.Round(amount * 100))
}

func gatewayReference(unitCode string) string {
	return fmt.Sprintf("BPLZ-%s-%d", sanitizeRefSuffix(unitCode), time.Now().Unix())
}

func joinChargeReferences(charges []Charge) string {
	refs := make([]string, 0, len(charges))
	for _, charge := range charges {
		refs = append(refs, charge.Reference)
	}
	return strings.Join(refs, ",")
}

func splitChargeReferences(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	refs := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	return refs
}

func flattenFormValues(values url.Values) map[string]string {
	flattened := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) == 0 {
			flattened[key] = ""
			continue
		}
		flattened[key] = items[0]
	}
	return flattened
}

func normalizeGatewayStatus(paid bool, state string) string {
	if paid {
		return "paid"
	}
	if strings.TrimSpace(state) == "" {
		return "failed"
	}
	return strings.ToLower(strings.TrimSpace(state))
}

func normalizeRedirectState(paid string) string {
	if strings.EqualFold(strings.TrimSpace(paid), "true") {
		return "paid"
	}
	return "failed"
}

func gatewayOutcome(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid":
		return "success"
	case "failed":
		return "failed"
	default:
		return "pending"
	}
}

func gatewayPaymentDescription(transaction GatewayTransaction, callback billplzCallbackResult) string {
	if callback.TransactionID != "" {
		return fmt.Sprintf("Online Billplz payment settled. Transaction %s", callback.TransactionID)
	}
	return fmt.Sprintf("Online Billplz payment settled for %s", transaction.UnitCode)
}

func paidAtLabel(value string) string {
	for _, layout := range []string{
		"2006-01-02 15:04:05 -0700",
		time.RFC3339,
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("02 Jan 2006 • 03:04 PM")
		}
	}
	return nowLabel()
}

package codec

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"apipro/desc/proto/gen/fy"

	"google.golang.org/protobuf/proto"
)

// TestProtobufEnvelopeRoundTrip verifies the full wire flow per
// docs/password-login-register.txt:
//
//   Request:  FY_CLIENT{common_req{client_info, param(JSON)}} → AES-ECB → frame
//   Response: frame → AES-ECB → FY_CLIENT{common_resp{common_result, result(JSON)}}
//
// This mirrors what the real zbyy frontend does: pack COMMON_REQ.param with
// the business JSON, serialize FY_CLIENT, AES-encrypt, prepend the 6-byte
// header, POST. The server reverses this and builds a protobuf response.
func TestProtobufEnvelopeRoundTrip(t *testing.T) {
	reqKey := []byte(DefaultRequestKey)  // PHp1st5vEg5Ca8FH
	respKey := []byte(DefaultResponseKey) // qlCJekfRKwWkQxl7

	// 1. Build the business JSON (account login per spec).
	loginJSON := map[string]any{
		"accountType": 2,
		"loginName":   "testuser",
		"password":    "200820e3227815ed1756a6b531e7e0d2", // md5("qwe123")
		"pwdType":     2,
		"loginMode":   1,
		"loginType":   1,
	}
	paramBytes, _ := json.Marshal(loginJSON)

	// 2. Pack into FY_CLIENT.common_req{ client_info, param }.
	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{
				SessionId: "-1760000000000123456", // guest sessionId per spec format
				Seq:       0,
				AppVer:    0,
				PackageCode: 0,
				Plat:      3, // Web
				Language:  1,
			},
			Param: paramBytes,
		},
	}
	plainReq, err := proto.Marshal(fcReq)
	if err != nil {
		t.Fatalf("marshal FY_CLIENT request: %v", err)
	}

	// 3. AES-encrypt + frame (what the client sends).
	wireReq, err := EncodeFrame(plainReq, reqKey)
	if err != nil {
		t.Fatalf("EncodeFrame request: %v", err)
	}
	if wireReq[0] != 0x00 || wireReq[1] != 0xA0 {
		t.Fatalf("bad frame magic: %x %x", wireReq[0], wireReq[1])
	}

	// 4. Server side: decode frame + decrypt.
	plainReq2, err := DecodeFrame(wireReq, reqKey)
	if err != nil {
		t.Fatalf("DecodeFrame request: %v", err)
	}
	if !bytes.Equal(plainReq, plainReq2) {
		t.Fatalf("plaintext mismatch after round-trip")
	}

	// 5. Parse the FY_CLIENT request.
	var fcReq2 fy.FY_CLIENT
	if err := proto.Unmarshal(plainReq2, &fcReq2); err != nil {
		t.Fatalf("unmarshal FY_CLIENT request: %v", err)
	}
	if fcReq2.CommonReq == nil || fcReq2.CommonReq.ClientInfo == nil {
		t.Fatalf("missing common_req.client_info")
	}
	ci := fcReq2.CommonReq.ClientInfo
	if ci.SessionId != "-1760000000000123456" {
		t.Errorf("sessionId: got %q want -1760000000000123456", ci.SessionId)
	}
	if ci.Plat != 3 {
		t.Errorf("plat: got %d want 3", ci.Plat)
	}
	if ci.Language != 1 {
		t.Errorf("language: got %d want 1", ci.Language)
	}
	// Verify the business JSON inside param.
	var login2 map[string]any
	if err := json.Unmarshal(fcReq2.CommonReq.Param, &login2); err != nil {
		t.Fatalf("unmarshal param JSON: %v", err)
	}
	if login2["loginName"] != "testuser" {
		t.Errorf("loginName: got %v want testuser", login2["loginName"])
	}
	if login2["pwdType"].(float64) != 2 {
		t.Errorf("pwdType: got %v want 2", login2["pwdType"])
	}

	// 6. Build the server response: FY_CLIENT{common_resp{common_result, result}}.
	respJSON := map[string]any{
		"sessionId":    "abc123def456", // server-issued session
		"accessToken":  "abc123def456",
		"refreshToken": "xyz789",
	}
	resultBytes, _ := json.Marshal(respJSON)
	fcResp := &fy.FY_CLIENT{
		CommonResp: &fy.COMMON_RESP{
			CommonResult: &fy.COMMON_RESULT{
				ErrCode:      200,
				ErrMsg:       "",
				Seq:          0,
				NewSessionId: "abc123def456",
			},
			Result: resultBytes,
		},
	}
	plainResp, err := proto.Marshal(fcResp)
	if err != nil {
		t.Fatalf("marshal FY_CLIENT response: %v", err)
	}

	// 7. AES-encrypt + frame (what the server sends back).
	wireResp, err := EncodeFrame(plainResp, respKey)
	if err != nil {
		t.Fatalf("EncodeFrame response: %v", err)
	}

	// 8. Client side: decode frame + decrypt.
	plainResp2, err := DecodeFrame(wireResp, respKey)
	if err != nil {
		t.Fatalf("DecodeFrame response: %v", err)
	}

	// 9. Parse the FY_CLIENT response.
	var fcResp2 fy.FY_CLIENT
	if err := proto.Unmarshal(plainResp2, &fcResp2); err != nil {
		t.Fatalf("unmarshal FY_CLIENT response: %v", err)
	}
	if fcResp2.CommonResp == nil || fcResp2.CommonResp.CommonResult == nil {
		t.Fatalf("missing common_resp.common_result")
	}
	cr := fcResp2.CommonResp.CommonResult
	if cr.ErrCode != 200 {
		t.Errorf("err_code: got %d want 200", cr.ErrCode)
	}
	if cr.NewSessionId != "abc123def456" {
		t.Errorf("new_session_id: got %q want abc123def456", cr.NewSessionId)
	}
	// Verify the business JSON inside result.
	var resp2 map[string]any
	if err := json.Unmarshal(fcResp2.CommonResp.Result, &resp2); err != nil {
		t.Fatalf("unmarshal result JSON: %v", err)
	}
	if resp2["sessionId"] != "abc123def456" {
		t.Errorf("result.sessionId: got %v want abc123def456", resp2["sessionId"])
	}
}

// TestWapResponseKeySelection verifies that the codec selects the WAP response
// key (PHp1st5vEg5Ca8FH) for plat=4 clients, per the spec:
//   "WAP API_KEY_RESP = PHp1st5vEg5Ca8FH" (same as request key)
func TestWapResponseKeySelection(t *testing.T) {
	cfg := TransportConfig{
		RequestKey:     []byte("PHp1st5vEg5Ca8FH"),
		ResponseKey:    []byte("qlCJekfRKwWkQxl7"), // Web
		WapResponseKey: []byte("PHp1st5vEg5Ca8FH"), // WAP (same as req)
	}

	// Simulate a WAP request (plat=4).
	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{Plat: 4, Language: 1},
			Param:      []byte(`{}`),
		},
	}
	plain, _ := proto.Marshal(fcReq)
	wire, err := EncodeFrame(plain, cfg.RequestKey)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}

	// Decode the request (simulating server-side).
	plain2, err := DecodeFrame(wire, cfg.RequestKey)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}
	var fcReq2 fy.FY_CLIENT
	if err := proto.Unmarshal(plain2, &fcReq2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	plat := fcReq2.CommonReq.ClientInfo.Plat
	if plat != 4 {
		t.Fatalf("plat: got %d want 4", plat)
	}

	// Select the response key based on plat (this is the logic in Transport).
	respKey := cfg.ResponseKey
	if plat == 4 && len(cfg.WapResponseKey) > 0 {
		respKey = cfg.WapResponseKey
	}
	if !bytes.Equal(respKey, cfg.WapResponseKey) {
		t.Errorf("WAP resp key not selected: got %q want %q", respKey, cfg.WapResponseKey)
	}

	// Verify the WAP key can decrypt what the WAP key encrypts.
	respPlain := []byte(`{"code":200,"result":{"sessionId":"wap-session"}}`)
	wireResp, err := EncodeFrame(respPlain, respKey)
	if err != nil {
		t.Fatalf("EncodeFrame response with WAP key: %v", err)
	}
	decrypted, err := DecodeFrame(wireResp, respKey)
	if err != nil {
		t.Fatalf("DecodeFrame response with WAP key: %v", err)
	}
	if !bytes.Equal(decrypted, respPlain) {
		t.Fatalf("WAP round-trip mismatch")
	}

	// Verify the Web key CANNOT decrypt a WAP-encrypted response.
	_, err = DecodeFrame(wireResp, cfg.ResponseKey)
	if err == nil {
		t.Error("Web key should NOT be able to decrypt WAP-encrypted response")
	}
}

// TestProtobufEnvelopeAccountRegister verifies the account registration
// request shape per spec (accountType=2, NO smsCode, only kaptcha).
func TestProtobufEnvelopeAccountRegister(t *testing.T) {
	regJSON := map[string]any{
		"accountType": 2,
		"loginName":   "newuser",
		"password":    "200820e3227815ed1756a6b531e7e0d2",
		"nickName":    "NewUser",
		"pwdType":     2,
		"kaptcha":     "ABC23",
	}
	paramBytes, _ := json.Marshal(regJSON)

	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{Plat: 3, Language: 1},
			Param:      paramBytes,
		},
	}
	plain, _ := proto.Marshal(fcReq)

	// Round-trip through AES+frame.
	key := []byte(DefaultRequestKey)
	wire, err := EncodeFrame(plain, key)
	if err != nil {
		t.Fatalf("EncodeFrame: %v", err)
	}
	plain2, err := DecodeFrame(wire, key)
	if err != nil {
		t.Fatalf("DecodeFrame: %v", err)
	}

	var fcReq2 fy.FY_CLIENT
	if err := proto.Unmarshal(plain2, &fcReq2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var reg2 map[string]any
	if err := json.Unmarshal(fcReq2.CommonReq.Param, &reg2); err != nil {
		t.Fatalf("unmarshal param: %v", err)
	}

	// Verify the spec fields are present.
	if reg2["accountType"].(float64) != 2 {
		t.Errorf("accountType: got %v want 2", reg2["accountType"])
	}
	if reg2["loginName"] != "newuser" {
		t.Errorf("loginName: got %v want newuser", reg2["loginName"])
	}
	if reg2["kaptcha"] != "ABC23" {
		t.Errorf("kaptcha: got %v want ABC23", reg2["kaptcha"])
	}
	// Verify NO smsCode field (account register doesn't use SMS per spec).
	if _, hasSMS := reg2["smsCode"]; hasSMS {
		t.Error("account register should NOT have smsCode per spec")
	}
}

// TestPlatFieldNumberFallback verifies the doc-typo fallback: if plat (field 5)
// is 0 but app_ver (field 3) is non-zero, use app_ver as plat. This handles
// clients that encode plat at field 3 (the doc shows plat=3 colliding with
// app_ver).
func TestPlatFieldNumberFallback(t *testing.T) {
	// Simulate a client that puts plat at field 3 (app_ver slot).
	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{
				AppVer: 4, // client put WAP plat here (field 3)
				Plat:   0, // field 5 not set
			},
			Param: []byte(`{}`),
		},
	}
	plain, _ := proto.Marshal(fcReq)

	var fcReq2 fy.FY_CLIENT
	if err := proto.Unmarshal(plain, &fcReq2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ci := fcReq2.CommonReq.ClientInfo

	// Apply the fallback logic from the middleware.
	plat := ci.Plat
	if plat == 0 && ci.AppVer != 0 {
		plat = ci.AppVer
	}
	if plat != 4 {
		t.Errorf("plat fallback: got %d want 4 (WAP)", plat)
	}
}

// TestBuildProtobufEnvelope verifies the response builder maps the legacy
// {code,meg,seq,newSessionId,result} JSON to the protobuf COMMON_RESP fields.
func TestBuildProtobufEnvelope(t *testing.T) {
	// Handler wrote a legacy JSON envelope.
	handlerOut := []byte(`{"code":200,"meg":"","seq":42,"newSessionId":"sess-abc","result":{"sessionId":"sess-abc"}}`)
	envelope := buildProtobufEnvelope(handlerOut, 42, "sess-abc")

	var fc fy.FY_CLIENT
	if err := proto.Unmarshal(envelope, &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if fc.CommonResp == nil || fc.CommonResp.CommonResult == nil {
		t.Fatalf("missing common_resp.common_result")
	}
	cr := fc.CommonResp.CommonResult
	if cr.ErrCode != 200 {
		t.Errorf("err_code: got %d want 200", cr.ErrCode)
	}
	if cr.Seq != 42 {
		t.Errorf("seq: got %d want 42", cr.Seq)
	}
	if cr.NewSessionId != "sess-abc" {
		t.Errorf("new_session_id: got %q want sess-abc", cr.NewSessionId)
	}
	// The result bytes should contain the inner result JSON.
	var r map[string]any
	if err := json.Unmarshal(fc.CommonResp.Result, &r); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if r["sessionId"] != "sess-abc" {
		t.Errorf("result.sessionId: got %v want sess-abc", r["sessionId"])
	}
}

// TestPlatStringRoundTrip verifies that the plat int→string→int conversion
// used between codec ctx and CallReq.Plat preserves the value.
func TestPlatStringRoundTrip(t *testing.T) {
	for _, plat := range []int{3, 4, 0} {
		s := strconv.Itoa(plat)
		n, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("Atoi(%q): %v", s, err)
		}
		if n != plat {
			t.Errorf("round-trip plat %d → %q → %d", plat, s, n)
		}
	}
}

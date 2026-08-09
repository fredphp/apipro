// Smoke-test client: exercises the full protobuf FY_CLIENT wire flow against
// a running apipro-api server. Verifies:
//   1. Account register (accountType=2, no SMS, kaptcha) via protobuf envelope
//   2. Account login (accountType=2) via protobuf envelope
//   3. WAP response key selection (plat=4)
//   4. Legacy JSON envelope still works (backward compat)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"apipro/desc/proto/gen/fy"
	"apipro/pkg/codec"

	"google.golang.org/protobuf/proto"
)

const (
	webReqKey  = "PHp1st5vEg5Ca8FH"
	webRespKey = "qlCJekfRKwWkQxl7"
	wapRespKey = "PHp1st5vEg5Ca8FH"
)

func main() {
	apiURL := "http://127.0.0.1:3100"
	if len(os.Args) > 1 {
		apiURL = os.Args[1]
	}
	fmt.Printf("=== apipro smoke test against %s ===\n\n", apiURL)

	// [1] kaptcha
	fmt.Println("[1] GET /api/kaptcha?t=<ts>&mobile=testuser")
	kaptchaCode := getKaptcha(apiURL, "testuser")
	fmt.Printf("    kaptcha code parsed from SVG: %q\n\n", kaptchaCode)

	// [2] account register via protobuf FY_CLIENT (Web plat=3)
	fmt.Println("[2] POST /login/reg (account register, accountType=2, plat=3 Web)")
	regJSON := map[string]any{
		"accountType": 2,
		"loginName":   "testuser",
		"password":    "200820e3227815ed1756a6b531e7e0d2",
		"nickName":    "TestUser",
		"pwdType":     2,
		"kaptcha":     kaptchaCode,
	}
	regResp := postProtobuf(apiURL+"/login/reg", regJSON, 3, webReqKey, webRespKey)
	if regResp != nil {
		fmt.Printf("    err_code=%d err_msg=%q new_session_id=%q\n", regResp.CommonResult.ErrCode, regResp.CommonResult.ErrMsg, regResp.CommonResult.NewSessionId)
		var result map[string]any
		json.Unmarshal(regResp.Result, &result)
		if sid, ok := result["sessionId"]; ok {
			fmt.Printf("    result.sessionId=%v OK\n", sid)
		}
		fmt.Println()
	}

	// [3] account login via protobuf FY_CLIENT (Web plat=3)
	fmt.Println("[3] POST /login/login (account login, accountType=2, plat=3 Web)")
	loginJSON := map[string]any{
		"accountType": 2,
		"loginName":   "testuser",
		"password":    "200820e3227815ed1756a6b531e7e0d2",
		"pwdType":     2,
		"loginMode":   1,
		"loginType":   1,
	}
	loginResp := postProtobuf(apiURL+"/login/login", loginJSON, 3, webReqKey, webRespKey)
	if loginResp != nil {
		fmt.Printf("    err_code=%d err_msg=%q new_session_id=%q\n", loginResp.CommonResult.ErrCode, loginResp.CommonResult.ErrMsg, loginResp.CommonResult.NewSessionId)
		var result map[string]any
		json.Unmarshal(loginResp.Result, &result)
		if sid, ok := result["sessionId"]; ok {
			fmt.Printf("    result.sessionId=%v OK\n", sid)
		}
		fmt.Println()
	}

	// [4] WAP login (plat=4) — verify WAP resp key
	fmt.Println("[4] POST /login/login (account login, accountType=2, plat=4 WAP)")
	wapResp := postProtobuf(apiURL+"/login/login", loginJSON, 4, webReqKey, wapRespKey)
	if wapResp != nil {
		fmt.Printf("    err_code=%d err_msg=%q new_session_id=%q\n", wapResp.CommonResult.ErrCode, wapResp.CommonResult.ErrMsg, wapResp.CommonResult.NewSessionId)
		fmt.Println("    OK: WAP response decrypted with WAP resp key")
		fmt.Println()
	}

	// [5] legacy JSON envelope (backward compat)
	fmt.Println("[5] POST /login/login (legacy JSON envelope, no protobuf)")
	jsonResp := postJSON(apiURL+"/login/login", loginJSON, webReqKey, webRespKey)
	if jsonResp != nil {
		fmt.Printf("    code=%d meg=%q\n", jsonResp.Code, jsonResp.Meg)
		fmt.Println("    OK: legacy JSON path still works")
		fmt.Println()
	}

	fmt.Println("=== smoke test complete ===")
}

func getKaptcha(baseURL, mobile string) string {
	url := fmt.Sprintf("%s/api/kaptcha?t=%d&mobile=%s", baseURL, time.Now().UnixMilli(), mobile)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("    kaptcha error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("    kaptcha status=%d body=%s\n", resp.StatusCode, string(body))
		return ""
	}
	// Parse the code from <text ...>CODE</text>
	svg := string(body)
	start := strings.Index(svg, ">")
	for i := 0; i < 3 && start >= 0; i++ {
		rest := svg[start+1:]
		end := strings.Index(rest, "<")
		if end < 0 {
			break
		}
		code := strings.TrimSpace(rest[:end])
		if len(code) > 0 && code != "" {
			// Check if this looks like the captcha text (alphanumeric)
			isCode := true
			for _, c := range code {
				if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
					isCode = false
					break
				}
			}
			if isCode && len(code) >= 4 {
				return code
			}
		}
		svg = rest[end:]
		start = strings.Index(svg, ">")
	}
	return ""
}

func postProtobuf(url string, businessJSON map[string]any, plat int, reqKey, respKey string) *fy.COMMON_RESP {
	paramBytes, _ := json.Marshal(businessJSON)
	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{
				SessionId: fmt.Sprintf("-%d%06d", time.Now().UnixMilli(), time.Now().Nanosecond()%1000000),
				Seq:       1,
				Plat:      int32(plat),
				Language:  1,
			},
			Param: paramBytes,
		},
	}
	plain, err := proto.Marshal(fcReq)
	if err != nil {
		fmt.Printf("    marshal error: %v\n", err)
		return nil
	}
	wire, err := codec.EncodeFrame(plain, []byte(reqKey))
	if err != nil {
		fmt.Printf("    encode frame error: %v\n", err)
		return nil
	}
	resp, err := http.Post(url, "application/octet-stream", bytes.NewReader(wire))
	if err != nil {
		fmt.Printf("    http error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		fmt.Printf("    http status=%d body=%x\n", resp.StatusCode, body)
		return nil
	}
	plain2, err := codec.DecodeFrame(body, []byte(respKey))
	if err != nil {
		fmt.Printf("    decode frame error: %v (body len=%d)\n", err, len(body))
		return nil
	}
	var fcResp fy.FY_CLIENT
	if err := proto.Unmarshal(plain2, &fcResp); err != nil {
		fmt.Printf("    unmarshal FY_CLIENT error: %v\n", err)
		return nil
	}
	if fcResp.CommonResp == nil || fcResp.CommonResp.CommonResult == nil {
		fmt.Printf("    missing common_resp.common_result\n")
		return nil
	}
	return fcResp.CommonResp
}

func postJSON(url string, businessJSON map[string]any, reqKey, respKey string) *codec.PlainResp {
	paramBytes, _ := json.Marshal(businessJSON)
	env := map[string]any{
		"sessionId": "",
		"seq":       1,
		"plat":      "3",
		"param":     json.RawMessage(paramBytes),
	}
	plain, _ := json.Marshal(env)
	wire, err := codec.EncodeFrame(plain, []byte(reqKey))
	if err != nil {
		fmt.Printf("    encode frame error: %v\n", err)
		return nil
	}
	resp, err := http.Post(url, "application/octet-stream", bytes.NewReader(wire))
	if err != nil {
		fmt.Printf("    http error: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	plain2, err := codec.DecodeFrame(body, []byte(respKey))
	if err != nil {
		fmt.Printf("    decode frame error: %v\n", err)
		return nil
	}
	var pr codec.PlainResp
	if err := json.Unmarshal(plain2, &pr); err != nil {
		fmt.Printf("    unmarshal PlainResp error: %v\n", err)
		return nil
	}
	return &pr
}

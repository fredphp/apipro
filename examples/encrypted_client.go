// 简易加密调用客户端 —— 演示如何用 apipro 的 codec 包发送加密请求
// 用法: go run examples/encrypted_client.go
//
// 本示例调用 /login/login 接口（testuser 账号已由 smoketest 注册），
// 完整演示：构造业务 JSON → protobuf 信封 → AES 加密 → HTTP 请求 → AES 解密 → 解析响应
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"apipro/desc/proto/gen/fy"
	"apipro/pkg/codec"

	"google.golang.org/protobuf/proto"
)

const (
	apiURL  = "http://127.0.0.1:3100"
	reqKey  = "PHp1st5vEg5Ca8FH" // 请求加密密钥（Web/WAP 通用）
	respKey = "qlCJekfRKwWkQxl7" // Web 响应密钥（plat=3）
	// wapRespKey = "PHp1st5vEg5Ca8FH" // WAP 响应密钥（plat=4，与请求相同）
)

func main() {
	// 1. 构造登录业务 JSON
	businessJSON, _ := json.Marshal(map[string]any{
		"accountType": 2,
		"loginName":   "testuser",                            // smoketest 已注册的账号
		"password":    "200820e3227815ed1756a6b531e7e0d2",  // md5("qwe123")
		"pwdType":     2,
		"loginMode":   1,
		"loginType":   1,
	})
	fmt.Printf("=== 步骤 1: 构造业务 JSON ===\n%s\n\n", businessJSON)

	// 2. 用 protobuf FY_CLIENT 信封包装
	fcReq := &fy.FY_CLIENT{
		CommonReq: &fy.COMMON_REQ{
			ClientInfo: &fy.CLIENT_INFO{
				SessionId: fmt.Sprintf("-%d", time.Now().UnixMilli()),
				Seq:       1,
				Plat:      3, // 3=Web, 4=WAP
				Language:  1, // 1=中文
			},
			Param: businessJSON,
		},
	}
	plain, _ := proto.Marshal(fcReq)
	fmt.Printf("=== 步骤 2: protobuf 序列化 ===\n%d bytes (FY_CLIENT 信封明文)\n\n", len(plain))

	// 3. AES-128-ECB 加密 + 帧封装（6 字节帧头 + 密文）
	wire, err := codec.EncodeFrame(plain, []byte(reqKey))
	if err != nil {
		panic(err)
	}
	fmt.Printf("=== 步骤 3: AES-ECB 加密 + 帧封装 ===\n%d bytes (6 字节帧头 + 密文), 帧头: % x\n\n", len(wire), wire[:6])

	// 4. 发送 HTTP 请求
	resp, err := http.Post(apiURL+"/login/login", "application/octet-stream", bytes.NewReader(wire))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("=== 步骤 4: HTTP POST /login/login ===\nHTTP %d, 响应 %d bytes (二进制密文)\n", resp.StatusCode, len(body))
	fmt.Printf("响应前 16 字节: % x\n\n", body[:min(16, len(body))])

	// 5. 解密响应
	plain2, err := codec.DecodeFrame(body, []byte(respKey))
	if err != nil {
		panic(err)
	}
	var fcResp fy.FY_CLIENT
	if err := proto.Unmarshal(plain2, &fcResp); err != nil {
		panic(err)
	}
	cr := fcResp.CommonResp.CommonResult
	fmt.Printf("=== 步骤 5: 解密响应 → 解析 protobuf ===\n")
	fmt.Printf("err_code:       %d\n", cr.ErrCode)
	fmt.Printf("err_msg:        %q\n", cr.ErrMsg)
	fmt.Printf("seq:            %d\n", cr.Seq)
	fmt.Printf("new_session_id: %q  ← 这就是 accessToken\n", cr.NewSessionId)
	fmt.Printf("result (JSON):  %s\n\n", string(fcResp.CommonResp.Result))

	// 6. 解析 result 里的业务字段
	if len(fcResp.CommonResp.Result) > 0 {
		var result map[string]any
		_ = json.Unmarshal(fcResp.CommonResp.Result, &result)
		fmt.Printf("=== 步骤 6: 解析 result 业务字段 ===\n")
		fmt.Printf("accessToken:  %v\n", result["accessToken"])
		fmt.Printf("sessionId:    %v  ← 必须等于 new_session_id\n", result["sessionId"])
		fmt.Printf("refreshToken: %v\n", result["refreshToken"])
		if ui, ok := result["userInfo"].(map[string]any); ok {
			fmt.Printf("userInfo.uid:      %v\n", ui["uid"])
			fmt.Printf("userInfo.nickName: %v\n", ui["nickName"])
			fmt.Printf("userInfo.loginName: %v\n", ui["loginName"])
		}
		fmt.Printf("\n✓ SPEC CHECK: new_session_id == result.sessionId: %v\n", cr.NewSessionId == result["sessionId"])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

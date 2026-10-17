// Package privileged is the small allowlisted protocol shared by UI/helper.
package privileged

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"xiaomi-iot-linux/internal/iot"
	"xiaomi-iot-linux/internal/platform"
)

const MaxRequest = 4096

type Request struct {
	Action   string `json:"action"`
	SSID     string `json:"ssid,omitempty"`
	Password string `json:"password,omitempty"`
}
type Response struct {
	OK      bool        `json:"ok"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
	Status  *iot.Status `json:"status,omitempty"`
}

func Parse(data []byte) (Request, error) {
	var r Request
	if len(data) > MaxRequest {
		return r, errors.New("请求过长")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return Request{}, errors.New("请求格式不正确")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return Request{}, errors.New("只允许一个请求")
	}
	switch r.Action {
	case "status", "pair", "reopen-window":
		if r.SSID != "" || r.Password != "" {
			return Request{}, errors.New("该操作不接受额外参数")
		}
	case "configure":
		if e := iot.ValidateWiFi(r.SSID, r.Password); e != nil {
			return Request{}, e
		}
	default:
		return Request{}, errors.New("不支持的操作")
	}
	return r, nil
}

func Execute(ctx context.Context, r Request) (resp Response) {
	var err error
	if err = iot.Guard(); err != nil {
		return Response{Error: err.Error()}
	}
	switch r.Action {
	case "status", "configure", "pair":
		err = iot.WithDevice(ctx, func(m *iot.Mailbox) error {
			var s iot.Status
			var e error
			switch r.Action {
			case "status":
				s, e = m.Status(ctx)
			case "configure":
				s, e = m.Configure(ctx, r.SSID, r.Password)
			case "pair":
				s, e = m.Pair(ctx)
			}
			if e == nil {
				resp.Status = &s
			}
			return e
		})
	case "reopen-window":
		var s iot.Status
		s, err = platform.ReopenWindow(ctx)
		if err == nil {
			resp.Status = &s
		}
	default:
		err = fmt.Errorf("不支持的操作")
	}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.OK = true
	switch r.Action {
	case "configure":
		if resp.Status.WiFiCode == 5 {
			resp.Message = "网络已保存，模块已连接小米云。"
		} else {
			resp.Message = "网络已保存；模块尚未连接小米云，请检查网络或密码。"
		}
	case "pair":
		if resp.Status.Bound {
			resp.Message = "设备已经绑定米家，无需重新添加。"
		} else {
			resp.Message = "添加窗口已打开，请在手机米家中扫描附近设备。"
		}
	case "reopen-window":
		resp.Message = "已重新打开配网窗口。"
	}
	return resp
}

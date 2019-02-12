package main

import (
	"bytes"
	"encoding/json"
	"github.com/alecthomas/log4go"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type TokenResponse struct {
	Token   string `json:"access_token"`
	Expires int    `json:"expires_in"`
}

type WechatClient struct {
	token        string
	tokenExpires time.Time
	corpId       string
	secret       string
}

func NewWechatClient(corp, secret string) *WechatClient {
	return &WechatClient{
		corpId: corp,
		secret: secret,
		token:  "",
	}
}

func (w *WechatClient) reqToken() string {
	if w.token != "" && w.tokenExpires.After(time.Now()) {
		return w.token
	}

	client := &http.Client{}

	url_params := &url.Values{}
	url_params.Add("corpid", w.corpId)
	url_params.Add("corpsecret", w.secret)

	u := url.URL{
		Scheme:   "https",
		Host:     "qyapi.weixin.qq.com",
		Path:     "/cgi-bin/gettoken",
		RawQuery: url_params.Encode(),
	}

	if req, err := http.NewRequest("GET", u.String(), nil); err == nil {
		resp, err := client.Do(req)

		if err == nil {
			defer resp.Body.Close()
			o, _ := httputil.DumpResponse(resp, true)
			log4go.Debug("resp %s", o)

			dec := json.NewDecoder(resp.Body)
			m := TokenResponse{}
			dec.Decode(&m)

			w.tokenExpires = time.Now().Add(time.Duration(m.Expires) * time.Second)
			log4go.Info("got token: %s", m.Token)
			return m.Token
		} else {
			log4go.Warn("require token error :%v", err)
		}
	}

	return ""
}

func (w *WechatClient) SendText(user, party, tag, content string, agentid int) {
	client := &http.Client{}
	m := make(map[string]interface{})

	m["touser"] = user
	m["toparty"] = party
	m["totag"] = tag

	m["msgtype"] = "text"
	m["agentid"] = agentid
	m["text"] = map[string]string{
		"content": content,
	}

	token := w.reqToken()

	url_params := &url.Values{}
	url_params.Add("access_token", token)

	u := url.URL{
		Scheme:   "https",
		Host:     "qyapi.weixin.qq.com",
		Path:     "/cgi-bin/message/send",
		RawQuery: url_params.Encode(),
	}

	b, _ := json.Marshal(m)

	b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
	b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
	b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)

	if req, err := http.NewRequest("POST", u.String(), bytes.NewReader(b)); err == nil {
		resp, err := client.Do(req)

		if err != nil {
			log4go.Warn("post message error : %v", err)
			return
		} else {
			r, _ := httputil.DumpRequest(req, true)
			log4go.Info("req %q, body %s", r, b)
			o, _ := httputil.DumpResponse(resp, true)
			log4go.Info("resp %q", o)

			log4go.Info("send message success")
		}
	}
}

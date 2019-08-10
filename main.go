package main

import (
	"encoding/json"
	"flag"
	"fmt"
    "bytes"
	"os"
	"path"
	"strings"

	"net/http"
	"net/http/httputil"
	"net/smtp"

	"github.com/alecthomas/log4go"
)

type Configuration struct {
	Debug bool `json:"debug"`
	Log   struct {
		Path    string `json:"path"`
		Name    string `json:"name"`
		Level   string `json:"level"`
		Console bool   `json:"console"`
	} `json:"log"`
	Gate struct {
		Addr string `json:"addr"`
	} `json:"gate"`
	Mail struct {
		Smtp struct {
			Addr     string `json:"addr"`
			Username string `json:"username"`
			Password string `json:"password"`
			From     string `json:"from"`
		} `json:"smtp"`
	} `json:"mail"`
	Wechat struct {
		Corp    string `json:"corp_id"`
		Secret  string `json:"secret"`
		AgentId int    `json:"agent_id"`
	}
    WechatRobot struct {
        Hook    string `json:"hook"`
    } `json:"wechat_robot"`
}

const (
	LOG_PREFIX = "mms"
)

var (
	VERSION   string
	Config    Configuration
	Wechat    *WechatClient
	level_map = map[string]log4go.Level{
		"DEBUG": log4go.DEBUG,
		"INFO":  log4go.INFO,
		"ERROR": log4go.ERROR,
	}
)

func WriteJsonOk(w http.ResponseWriter, body interface{}) {
	w.WriteHeader(http.StatusOK)

	je := json.NewEncoder(w)
	if body != nil {
		je.Encode(body)
	} else {
		je.Encode(map[string]interface{}{
			"message": "ok",
		})
	}
}

func WriteJsonError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code / 100)
	je := json.NewEncoder(w)
	je.Encode(map[string]interface{}{
		"code":    code,
		"message": msg,
	})
}

func emailHandler(w http.ResponseWriter, r *http.Request) {
	if Config.Debug {
		if d, err := httputil.DumpRequest(r, false); err != nil {
			log4go.Warn("dump error : %v", err)

		} else {
			log4go.Info("OnEmailRequest:", string(d))
		}
	}

	content := r.FormValue("content")
	tos := r.FormValue("tos")
	subject := r.FormValue("subject")

	if content == "" || tos == "" || subject == "" {
		WriteJsonError(w, 40001, "content or tos or subject error")
		return
	}

	auth := smtp.PlainAuth("", Config.Mail.Smtp.Username,
		Config.Mail.Smtp.Password, Config.Mail.Smtp.Addr)

	body := fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain;charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n",
		Config.Mail.Smtp.Username,
		tos,
		subject,
		content)

	c, err := smtp.Dial(Config.Mail.Smtp.Addr + ":25")
	if err != nil {
		log4go.Warn(err)
		WriteJsonError(w, 50001, fmt.Sprintf("dial error:%v", err))
		return
	}

	if err := c.Auth(auth); err != nil {
		log4go.Warn(err)
		WriteJsonError(w, 50001, fmt.Sprintf("auth error:%v", err))
		return
	}

	if err := c.Mail(Config.Mail.Smtp.From); err != nil {
		log4go.Warn(err)
		WriteJsonError(w, 50001, fmt.Sprintf("dial error:%v", err))
		return
	}

	for _, to := range strings.Split(tos, ",") {
		if err := c.Rcpt(to); err != nil {
			log4go.Warn(err)
			WriteJsonError(w, 50001, fmt.Sprintf("rcpt error:%v", err))
			return
		}
	}

	wc, err := c.Data()
	if err != nil {
		log4go.Warn(err)
		WriteJsonError(w, 50001, fmt.Sprintf("get data error:%v", err))
		return
	}

	_, err = fmt.Fprintf(wc, body)
	if err != nil {
		log4go.Warn(err)
		WriteJsonError(w, 50001, fmt.Sprintf("printf error:%v", err))
		return
	}

	err = wc.Close()
	if err != nil {
		log4go.Warn("close error", err)
		WriteJsonError(w, 50001, fmt.Sprintf("close error:%v", err))
		return
	}

	// Send the QUIT command and close the connection.
	err = c.Quit()
	if err != nil {
		log4go.Warn("Quit error : %v", err)
		WriteJsonError(w, 50001, fmt.Sprintf("quit error:%v", err))
	} else {
		log4go.Info("send success [%s][%s]", tos[0], body)
		WriteJsonOk(w, nil)
	}
}

func wechatHandler(w http.ResponseWriter, r *http.Request) {
	if Config.Debug {
		if d, err := httputil.DumpRequest(r, false); err != nil {
		} else {
			log4go.Info("OnSMSRequest:", string(d))
		}
	}

	content := r.FormValue("content")
	tos := r.FormValue("tos")
	if content == "" || tos == "" {
		WriteJsonError(w, 40001, "content or tos error")
		return
	}
	users := strings.Join(strings.Split(tos, ","), "|")

	Wechat.SendText(users, "", "", content, Config.Wechat.AgentId)
}

func smsHandler(w http.ResponseWriter, r *http.Request) {
    // send sms here
}

func wechatRobotHandler(w http.ResponseWriter, r *http.Request) {
	content := r.FormValue("content")
	tos := r.FormValue("tos")
	if content == "" || tos == "" {
		WriteJsonError(w, 40001, "content or tos error")
		return
	}

	client := &http.Client{}
    m := make(map[string]interface{})
    m["msgtype"] = "text"
    m["text"] = map[string]string {
        "content": content,
    }

    b, _ := json.Marshal(m)
    if req, err := http.NewRequest("POST", Config.WechatRobot.Hook, bytes.NewReader(b)); err == nil {
        req.Header.Set("Content-Type", "application/json")
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

func main() {
	var err error
	var conffile *os.File
	var level log4go.Level
	var ok bool

	showVersion := flag.Bool("v", false, "show version and exit")
	conf_file := flag.String("c", "config.json", "config file name")
	flag.Parse()

	if *showVersion {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	conffile, err = os.Open(*conf_file)
	if err != nil {
		log4go.Error("open file %s error", *conf_file)
		os.Exit(-1)
	}

	jd := json.NewDecoder(conffile)
	if err = jd.Decode(&Config); err != nil {
		log4go.Error("decode error %v", err)
		os.Exit(-1)
	}

	logfilepath := path.Join(Config.Log.Path, Config.Log.Name)

	if level, ok = level_map[Config.Log.Level]; !ok {
		level = log4go.INFO
	}

	if Config.Log.Console {
		log4go.Global.AddFilter("stdout", level, log4go.NewConsoleLogWriter())
	}

	log4go.Global.AddFilter("log", level, log4go.NewFileLogWriter(logfilepath, false))

	Wechat = NewWechatClient(Config.Wechat.Corp, Config.Wechat.Secret)

	//http.HandleFunc("/sms", wechatHandler)
	//http.HandleFunc("/mail", emailHandler)
	http.HandleFunc("/sms", wechatRobotHandler)

	log4go.Info("server starts at : %s", Config.Gate.Addr)
	log4go.Error(http.ListenAndServe(Config.Gate.Addr, nil))
}

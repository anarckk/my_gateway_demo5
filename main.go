package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	. "github.com/anarckk/my_gateway_demo5/src"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	configFile := flag.String("config", "config/config.yaml", "配置文件")
	flag.Parse()
	log.Println("配置文件: " + *configFile)

	yamlConfig := ParseConfig(*configFile)

	// http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	fs := http.FileServer(http.Dir("static/"))
	http.Handle("/static/", http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 记录请求信息
		log.Printf("Request Method: %s, URL: %s\n", r.Method, r.URL.Path)
		// 调用文件服务器进行处理
		fs.ServeHTTP(w, r)
	})))

	var redisController = RedisController{}
	redisController.Init(yamlConfig.Redis.Address)

	var services = yamlConfig.ReverseProxies
	var svrReverseProxyMap = make(map[string]*httputil.ReverseProxy)
	for _, svr := range services {
		_svr := svr
		svrURL, _ := url.Parse(_svr.Address)
		svrProxy := httputil.NewSingleHostReverseProxy(svrURL)
		svrProxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		svrProxy.ModifyResponse = func(r *http.Response) error {
			r.Header.Del("X-Frame-Options")
			r.Header.Add("X-Frame-Options", "SAMEORIGIN")
			return nil
		}
		svrReverseProxyMap[_svr.Name] = svrProxy
		http.HandleFunc("/"+_svr.Name, func(w http.ResponseWriter, r *http.Request) {
			cookies := r.Cookies()
			var allowUserId *http.Cookie
			for _, cookie := range cookies {
				if cookie.Name == "allow-user-id" {
					allowUserId = cookie
				}
			}
			if allowUserId == nil {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, "you are not invited!")
				log.Printf("Request Method: %s, URL: %s, 拒绝,allowUserId is nil\n", r.Method, r.URL.Path)
				return
			}
			// 检查是否是受邀请用户
			ok, err := redisController.CheckUser(context.TODO(), allowUserId.Value)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, "you are not invited!")
				log.Printf("Request allowUserId: %s, Method: %s, URL: %s, 拒绝,不是一个合法的用户\n", allowUserId.Value, r.Method, r.URL.Path)
				return
			}

			// 检查是否有权访问此服务
			ok = CheckAuthorization(_svr.Name, allowUserId.Value)
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintln(w, "you are not invited!")
				log.Printf("Request allowUserId: %s, Method: %s, URL: %s, 拒绝,无权限\n", allowUserId.Value, r.Method, r.URL.Path)
				return
			}
			log.Printf("Request allowUserId: %s, Method: %s, URL: %s\n", allowUserId.Value, r.Method, r.URL.Path)

			ck := http.Cookie{
				Name:     "my-service",
				Value:    url.QueryEscape(_svr.Name),
				Path:     "/",
				HttpOnly: false,
				// Expires:  time.Now().AddDate(10, 0, 0),
				MaxAge: 10 * 365 * 24 * 60 * 60,
			}
			w.Header().Add("Set-Cookie", ck.String())
			w.Header().Add("X-Frame-Options", "deny")
			w.Header().Add("Cache-Control", "no-cache")
			w.Header().Add("Pragma", "no-cache")
			w.Header().Add("Expires", "0")
			// 解析指定文件生成模板对象
			// 关于模板渲染 https://zhuanlan.zhihu.com/p/299048675
			tmpl, err := template.ParseFiles("tmpl/svr.tmpl")
			if err != nil {
				log.Printf("Request allowUserId: %s, Method: %s, URL: %s, create template failed, err: %s", allowUserId.Value, r.Method, r.URL.Path, err)
				return
			}
			// 利用给定数据渲染模板, 并将结果写入w
			tmpl.Execute(w, _svr.Name)
		})
	}
	http.HandleFunc("/checkInviteCode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintln(w, "server refuse you request! just allow post")
			log.Printf("Request Method: %s, URL: %s, 拒绝,因为仅接受post请求\n", r.Method, r.URL.Path)
			return
		}
		var requestBody = make(map[string]string)
		{
			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			err = json.Unmarshal(data, &requestBody)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		inviteCode, ok := requestBody["inviteCode"]
		if !ok {
			var resp = make(map[string]interface{})
			resp["ok"] = 0
			b, _ := json.Marshal(resp)
			w.Write(b)
			log.Printf("Request Method: %s, URL: %s, inviteCode不存在\n", r.Method, r.URL.Path)
			return
		}
		if inviteCode == "" {
			var resp = make(map[string]interface{})
			resp["ok"] = 0
			b, _ := json.Marshal(resp)
			w.Write(b)
			log.Printf("Request Method: %s, URL: %s, inviteCode是空字符串\n", r.Method, r.URL.Path)
			return
		}

		log.Printf("Request inviteCode: %s, Method: %s, URL: %s\n", inviteCode, r.Method, r.URL.Path)

		ok, err := redisController.CheckInviteCode(context.TODO(), inviteCode)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if ok {
			userSize, err := redisController.UserSize(context.TODO())
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			allowUserId := fmt.Sprintf("%d-%s", userSize+1, inviteCode)
			err = redisController.AddUser(context.TODO(), allowUserId)
			if err != nil {
				log.Println(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			allowUserIdCookie := http.Cookie{
				Name:     "allow-user-id",
				Value:    allowUserId,
				Path:     "/",
				HttpOnly: true,
				// Expires:  time.Now().AddDate(10, 0, 0),
				MaxAge: 10 * 365 * 24 * 60 * 60,
			}
			w.Header().Add("Set-Cookie", allowUserIdCookie.String())
			var resp = make(map[string]interface{})
			resp["ok"] = 1
			b, _ := json.Marshal(resp)
			w.Write(b)
			log.Printf("Request allowUserId: %s, Method: %s, URL: %s, 验证通过\n", allowUserId, r.Method, r.URL.Path)
		} else {
			var resp = make(map[string]interface{})
			resp["ok"] = 0
			b, _ := json.Marshal(resp)
			w.Write(b)
			log.Printf("Request Method: %s, URL: %s, 验证拒绝\n", r.Method, r.URL.Path)
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cookies := r.Cookies()
		var myService *http.Cookie
		var allowUserId *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == "my-service" {
				myService = cookie
			}
			if cookie.Name == "allow-user-id" {
				allowUserId = cookie
			}
		}
		if allowUserId == nil {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "you are not invited!")
			log.Printf("Request Method: %s, URL: %s, 拒绝,allowUserId is nil\n", r.Method, r.URL.Path)
			return
		}
		if myService == nil {
			fmt.Fprintln(w, "you address is wrong!")
			log.Printf("Request Method: %s, URL: %s, 拒绝,myService is nil\n", r.Method, r.URL.Path)
			return
		}
		// 检查是否是受邀请用户
		ok, err := redisController.CheckUser(context.TODO(), allowUserId.Value)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "you are not invited!")
			log.Printf("Request allowUserId: %s, Method: %s, URL: %s, 不是一个合法的用户\n", allowUserId.Value, r.Method, r.URL.Path)
			return
		}
		// 检查是否有权访问此服务
		ok = CheckAuthorization(myService.Value, allowUserId.Value)
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "you are not invited!")
			log.Printf("Request allowUserId: %s, myService: %s, Method: %s, URL: %s, 无权访问此服务\n", allowUserId.Value, myService.Value, r.Method, r.URL.Path)
			return
		}

		log.Printf("Request AllowUserId: %s, myService: %s, Method: %s, URL: %s\n", allowUserId.Value, myService.Value, r.Method, r.URL.Path)

		if proxy, ok := svrReverseProxyMap[myService.Value]; ok {
			proxy.ServeHTTP(w, r)
		} else {
			log.Printf("Request AllowUserId: %s, myService: %s, Method: %s, URL: %s, 服务未配置!\n", allowUserId.Value, myService.Value, r.Method, r.URL.Path)
		}
	})

	log.Printf("Listening on :%s\n", yamlConfig.Port)
	err := http.ListenAndServeTLS(":"+yamlConfig.Port, yamlConfig.Tls.CrtFile, yamlConfig.Tls.KeyFile, nil)
	if err != nil {
		log.Println(err)
	}
}

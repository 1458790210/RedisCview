package main

import (
	"RedisCview/lib"
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/zserge/lorca"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

type Result struct {
	Code    int
	SubCode int
	Reason  string
	Data    interface{}
}

func OutputJson(w http.ResponseWriter, code int, subCode int, reason string, i interface{}) {
	out := &Result{code, subCode, reason, i}
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	_, _ = w.Write(b)
}

func handle() {
	var (
		auth      string
		host      string
		db        *string
		port      int
		flagParse int
	)

	flag.Parse()
	bio := bufio.NewReader(os.Stdin)
INITIAL:
	db = new(string)
	*db = "0"
	fmt.Println("程序启动后会自动打开浏览器并跳转至 127.0.0.1:9681")
	fmt.Println("输入：-c 列表, -s [n] 选择连接, -r [host port auth] 输入连接, -d [n] 删除连接")
	for {
		fmt.Print("RedisCview> ")
		input, _, err := bio.ReadLine()
		if err != nil {
			fmt.Println(err)
		}
		s := string(input)
		if len(input) == 0 {
		} else if s == "-c" {
			l := lib.QueryServers()
			fmt.Println("n", "host", "port")
			for index, value := range l.Servers {
				fmt.Println(index+1, value.Host, value.Port)
			}
			flagParse = 1
		} else if strings.Contains(s, "-s") {
			s = strings.TrimLeft(s, " ")
			array := strings.Fields(s)
			if len(array) < 2 {
				fmt.Println("RedisCview> 无效的参数")
				continue
			}
			i, _ := strconv.Atoi(array[1])
			l := lib.QueryServers()
			for index, value := range l.Servers {
				if index == i-1 {
					fmt.Println("连接：", value.Host, ":", value.Port)
					host = value.Host
					port = value.Port
					auth = value.Auth
					flagParse = 2
					goto REDIS
				}
			}
		} else if strings.Contains(s, "-d") {
			flagParse = 2
			s = strings.TrimLeft(s, " ")
			array := strings.Fields(s)
			if len(array) < 2 {
				fmt.Println("RedisCview> 无效的参数")
				continue
			}
			i, _ := strconv.Atoi(array[1])
			l := lib.QueryServers()
			for index, value := range l.Servers {
				if index == i-1 {
					_ = lib.DelCli(value.ID)
				}
			}
		} else if strings.Contains(s, "-r") {
			s = strings.TrimLeft(s, " ")
			array := strings.Fields(s)
			if len(array) < 3 {
				fmt.Println("RedisCview> 无效的参数")
				continue
			}
			host = array[1]
			port, _ = strconv.Atoi(array[2])
			if len(array) >= 4 {
				auth = array[3]
			}
			flagParse = 3
			goto REDIS
		} else {
			fmt.Println("RedisCview> 无效的参数")
		}
	}
REDIS:
	cli := lib.NewClient(host, port)
	err := cli.Connect()

	if err != nil {
		fmt.Println("无效的连接")
		goto INITIAL
	}

	if auth != "" {
		i := []byte("auth " + auth)
		r, err := cli.Result(i)
		if err != nil {
			fmt.Println(err)
		}

		f := bytes.Fields(i)
		for _, s := range r.Format() {
			if string(f[0]) == "auth" && s == "OK" && flagParse == 3 {
				p := strconv.Itoa(port)
				_ = lib.SaveCli(host, host, string(f[1]), p)
			}
		}
	}

	defer cli.Close()

	for {
		fmt.Printf("%s:"+*db+"> ", host)

		input, _, err := bio.ReadLine()
		if err != nil {
			fmt.Println(err)
		}
		if string(input) != "" {
			if string(input) == "exit" {
				goto INITIAL
			}

			reply, err := cli.Result(input)
			if err != nil {
				fmt.Println(err)
			}

			for _, s := range reply.Format() {
				fmt.Println(s)
			}

			fields := bytes.Fields(input)
			if string(fields[0]) == "select" {
				if len(fields) == 2 {
					*db = string(fields[1])
				}
			}
		} else {
			_, _ = cli.Result([]byte("ping"))
		}
	}
}

var (
	upgrader = websocket.Upgrader{
		// 读取存储空间大小
		ReadBufferSize: 1024,
		// 写入存储空间大小
		WriteBufferSize: 1024,
		// 允许跨域
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func wsHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err    error
		data   []byte
		c      *lib.Client
		wbsCon *websocket.Conn
		auth_  string
		host_  string
		port_  int
	)

	// 完成http应答，在httpheader中放下如下参数
	if wbsCon, err = upgrader.Upgrade(w, r, nil); err != nil {
		return // 获取连接失败直接返回
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	if id != 0 {
		l := lib.QueryServers()
		for _, value := range l.Servers {
			if value.ID == id {
				host_ = value.Host
				auth_ = value.Auth
				port_ = value.Port
			}
		}
	} else {
		goto ERR
	}

	c = lib.NewClient(host_, port_)
	err = c.Connect()

	if auth_ != "" {
		_, err := c.Result([]byte("auth " + auth_))
		if err != nil {
			fmt.Println(err)
		}
	}

	defer c.Close()

	for {
		// 只能发送Text, Binary 类型的数据,下划线意思是忽略这个变量.
		if _, data, err = wbsCon.ReadMessage(); err != nil {
			goto ERR // 跳转到关闭连接
		}

		if string(data) != "" {
			reply, err := c.Result(data)
			if err != nil {
				fmt.Println(err)
			}

			rst := ""
			for _, s := range reply.Format() {
				rst += s + "<br/>"
			}

			if err = wbsCon.WriteMessage(websocket.TextMessage, []byte(rst)); err != nil {
				goto ERR // 发送消息失败，关闭连接
			}
		} else {
			_, _ = c.Result([]byte("ping"))
		}
	}
ERR:
	wbsCon.Close()
}

func Try(fun func(), handler func(interface{})) {
	defer func() {
		if err := recover(); err != nil {
			handler(err)
		}
	}()
	fun()
}

func main() {
	// 设置 处理函数
	//http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//	// 解析模板
	//	t, _ := template.ParseFiles("index.html")
	//	// 设置模板数据
	//	data := map[string]interface{}{
	//		"List": []string{"Go", "PHP", "JavaScript"},
	//	}
	//	// 渲染模板，发送响应
	//	_ = t.Execute(w, data)
	//})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, AddForm)
		return
	})

	http.HandleFunc("/query_servers", func(w http.ResponseWriter, r *http.Request) {
		OutputJson(w, 1, 100, "操作成功", lib.QueryServers())
		return
	})

	http.HandleFunc("/new_cli", NewCli)
	http.HandleFunc("/save_cli", SaveCli)
	http.HandleFunc("/scan_page", ScanPage)
	http.HandleFunc("/select_key", SelectKey)
	http.HandleFunc("/update_value", UpdateValue)
	http.HandleFunc("/remove_key", Del)
	http.HandleFunc("/ttl_val", Expire)
	http.HandleFunc("/rename", Rename)
	http.HandleFunc("/add_key", AddKey)
	http.HandleFunc("/del_line", DelLine)
	http.HandleFunc("/test_btn", TestCli)
	http.HandleFunc("/del_cli", DelCli)
	http.HandleFunc("/info_cli", InfoCli)
	http.HandleFunc("/update_system_configs", UpdateSystemConfigs)

	//打开浏览器
	//cmd := exec.Command("explorer", "http://127.0.0.1:9681")
	//err := cmd.Start()
	//if err != nil {
	//	fmt.Println(err.Error())
	//}

	Try(func() {
		go handle()
		http.HandleFunc("/ws", wsHandler)
	}, func(err interface{}) {

	})

	//使用桌面程序
	_ = http.ListenAndServe(":9681", nil)
	ui, _ := lorca.New("http://127.0.0.1:9681", "", 1600, 1020, "--disable-sync", " --disable-translate")
	chaSignal := make(chan os.Signal, 1)
	signal.Notify(chaSignal, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-ui.Done():
	case <-chaSignal:
	}
	ui.Close()
}

func NewCli(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 101, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" {
		OutputJson(w, 0, 102, "请选择连接", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))

	rst, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 103, "连接失败", nil)
	} else {
		if db >= 0 {
			ret, err := lib.QueryScan(rst, 0, "*")
			if err != nil {
				OutputJson(w, 0, 104, "选择数据库失败", nil)
			} else {
				OutputJson(w, 1, 105, "操作成功", ret)
			}
		} else {
			OutputJson(w, 1, 106, "操作成功", nil)
		}
	}

	return
}

func TestCli(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 107, "参数错误", nil)
		return
	}

	if r.FormValue("address") == "" || r.FormValue("port") == "" {
		OutputJson(w, 0, 108, "请输入参数", nil)
		return
	}

	name := r.FormValue("name")
	address := r.FormValue("address")
	password := r.FormValue("password")
	port := r.FormValue("port")

	err = lib.TestCli(name, address, password, port)

	if err != nil {
		OutputJson(w, 0, 109, "连接失败", nil)
	} else {
		OutputJson(w, 1, 110, "连接成功", nil)
	}

	return
}

func ScanPage(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 111, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("iterate") == "" {
		OutputJson(w, 0, 112, "请选择数据库", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	iterate, _ := strconv.Atoi(r.FormValue("iterate"))
	match := r.FormValue("match")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 113, "连接失败", nil)
	} else {
		rst, err := lib.QueryScan(cli, iterate, match)
		if err != nil {
			OutputJson(w, 0, 114, "翻页失败", nil)
		} else {
			OutputJson(w, 1, 115, "操作成功", rst)
		}
	}
}

func Expire(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 116, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 117, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	ttl, _ := strconv.Atoi(r.FormValue("ttl"))
	key := r.FormValue("key")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 118, "连接失败", nil)
	} else {
		err := lib.Expire(key, ttl, cli)
		if err != nil {
			OutputJson(w, 0, 119, "操作失败", nil)
		} else {
			OutputJson(w, 1, 120, "操作成功", nil)
		}
	}
}

func Rename(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 121, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 122, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	name := r.FormValue("name")
	key := r.FormValue("key")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 123, "连接失败", nil)
	} else {
		err := lib.Rename(key, name, cli)
		if err != nil {
			OutputJson(w, 0, 124, "操作失败", nil)
		} else {
			OutputJson(w, 1, 125, "操作成功", nil)
		}
	}
}

func Del(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 126, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 127, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	key := r.FormValue("key")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 128, "连接失败", nil)
	} else {
		err := lib.Del(key, cli)
		if err != nil {
			OutputJson(w, 0, 129, "删除失败", nil)
		} else {
			OutputJson(w, 1, 130, "操作成功", nil)
		}
	}
}

func DelLine(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 131, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 132, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	key := r.FormValue("key")
	val := r.FormValue("val")
	field := r.FormValue("field")
	t := r.FormValue("type")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 133, "连接失败", nil)
	} else {
		err := lib.DelLine(key, val, field, t, cli)
		if err != nil {
			OutputJson(w, 0, 134, "删除失败", nil)
		} else {
			OutputJson(w, 1, 135, "操作成功", nil)
		}
	}
}

func SelectKey(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 136, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 137, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	cursor, _ := strconv.Atoi(r.FormValue("cursor"))
	match := r.FormValue("match")
	key := r.FormValue("key")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 138, "连接失败", nil)
	} else {
		rst, err := lib.QueryKey(cli, key, uint64(cursor), match)
		if err != nil {
			OutputJson(w, 0, 139, "查询失败", nil)
		} else {
			OutputJson(w, 1, 140, "操作成功", rst)
		}
	}
}

func UpdateValue(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 142, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" {
		OutputJson(w, 0, 143, "请选择连接", nil)
		return
	}

	if r.FormValue("val") == "" || r.FormValue("key") == "" {
		OutputJson(w, 0, 144, "请选择key", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	index, _ := strconv.Atoi(r.FormValue("index"))
	val := r.FormValue("val")
	valOrignal := r.FormValue("valOrignal")
	key := r.FormValue("key")
	field := r.FormValue("field")
	fieldOrignal := r.FormValue("fieldOrignal")
	t := r.FormValue("type")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 145, "连接失败", nil)
	} else {
		err = lib.UpdateValue(valOrignal, fieldOrignal, val, key, field, t, int64(index), cli)
		if err != nil {
			OutputJson(w, 0, 146, "操作失败", nil)
		} else {
			OutputJson(w, 1, 147, "操作成功", nil)
		}
	}

	return
}

func AddKey(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 148, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" {
		OutputJson(w, 0, 149, "请选择连接", nil)
		return
	}

	if r.FormValue("key") == "" {
		OutputJson(w, 0, 150, "请输入key和value", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))
	val_1 := r.FormValue("val_1")
	val_2 := r.FormValue("val_2")
	key := r.FormValue("key")
	t := r.FormValue("type")

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 151, "连接失败", nil)
	} else {
		err = lib.AddKey(val_1, val_2, key, t, cli)
		if err != nil {
			OutputJson(w, 0, 152, "操作失败", nil)
		} else {
			OutputJson(w, 1, 153, "操作成功", nil)
		}
	}

	return
}

func SaveCli(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 154, "参数错误", nil)
		return
	}

	if r.FormValue("address") == "" || r.FormValue("port") == "" {
		OutputJson(w, 0, 155, "请输入参数", nil)
		return
	}

	name := r.FormValue("name")
	address := r.FormValue("address")
	password := r.FormValue("password")
	port := r.FormValue("port")

	err = lib.SaveCli(name, address, password, port)

	if err != nil {
		OutputJson(w, 0, 156, "连接失败", nil)
	} else {
		OutputJson(w, 1, 157, "连接成功", nil)
	}
	return
}

func DelCli(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 158, "参数错误", nil)
		return
	}

	if r.FormValue("id") == "" {
		OutputJson(w, 0, 159, "请选择连接", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))

	err = lib.DelCli(id)
	if err != nil {
		OutputJson(w, 0, 160, "删除失败", nil)
	} else {
		OutputJson(w, 1, 161, "操作成功", nil)
	}
}

func InfoCli(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 162, "参数错误", nil)
		return
	}

	if r.FormValue("db") == "" || r.FormValue("id") == "" {
		OutputJson(w, 0, 163, "请选择连接", nil)
		return
	}

	id, _ := strconv.Atoi(r.FormValue("id"))
	db, _ := strconv.Atoi(r.FormValue("db"))

	cli, err := lib.QueryCli(id, db)

	if err != nil {
		OutputJson(w, 0, 164, "连接失败", nil)
	} else {
		rst, err := lib.InfoCli(cli)
		if err != nil {
			OutputJson(w, 0, 165, "操作失败", nil)
		} else {
			OutputJson(w, 1, 166, "操作成功", rst)
		}
	}
}

func UpdateSystemConfigs(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		OutputJson(w, 0, 167, "参数错误", nil)
		return
	}

	if r.FormValue("data") == "" {
		OutputJson(w, 0, 168, "请输入参数", nil)
		return
	}

	data := r.FormValue("data")

	err = lib.SystemConfigs(data)
	if err != nil {
		OutputJson(w, 0, 169, "设置失败", nil)
	} else {
		OutputJson(w, 1, 170, "设置成功", nil)
	}
	return
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	// 1. 请求类型是application/x-www-form-urlencoded时解析form数据
	_ = r.ParseForm()
	fmt.Println(r.PostForm) // 打印form数据
	fmt.Println(r.PostForm.Get("name"), r.PostForm.Get("age"))
	// 2. 请求类型是application/json时从r.Body读取数据
	b, _ := ioutil.ReadAll(r.Body)
	fmt.Println("b", b)
}

const AddForm = `<!DOCTYPE html>
<html>
<head>
    <meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
    <meta name="title" content="RedisView"/>
    <meta name="description" content="RedisView-redis可视化"/>
    <meta name="keywords" content="可视化,redis,系统"/>
    <title>RedisCview</title>
    <link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/3.0.1/css/bootstrap.min.css">
    <script src="http://code.jquery.com/jquery-2.1.1.min.js"></script>
    <script src="https://maxcdn.bootstrapcdn.com/bootstrap/3.0.1/js/bootstrap.min.js"></script>
</head>
<body style="cursor: auto;" class="devpreview sourcepreview">
<div class="container">
    <div class="row clearfix">
        <div class="col-md-12 column" style="margin-top: 0;padding: 0;margin-bottom: 0">
            <nav class="navbar navbar-default navbar navbar-static-top" role="navigation"
                 style="margin: 0;border: none;">
                <div class="navbar-header">
                    <a class="navbar-brand">RedisCview</a>
                </div>
                <div class="collapse navbar-collapse">
                    <ul class="nav navbar-nav">
                        <li id="new_cli">
                            <a id="modal-new" href="#modal-container-new" role="button" data-toggle="modal">新建连接</a>
                        </li>
                    </ul>
                    <ul class="nav navbar-nav navbar-right" id="set_cli">
						<li><a role="button" target="_blank" href="https://github.com/1458790210/RedisCview">最新版本</a></li>
                        <li id="update_system_configs">
                            <a role="button" href="#modal-container-configs" data-toggle="modal">设置</a></li>
                    </ul>
                </div>
            </nav>

            <div class="modal fade" id="modal-container-configs" role="dialog"
                 aria-labelledby="myModalLabelnew" data-backdrop="static"
                 aria-hidden="true">
                <div class="modal-dialog">
                    <div class="modal-content">
                        <div class="modal-header">
                            <button type="button" class="close" data-dismiss="modal" aria-hidden="true">
                                ×
                            </button>
                            <h4 class="modal-title">系统设置</h4>
                        </div>
                        <div class="modal-body">
                            <form role="form">
                                <div class="form-group">
                                    <label for="name">keyScanLimits</label>
                                    <input type="text" class="form-control" id="keyScanLimits" value=""/>
                                </div>
                                <div class="form-group">
                                    <label for="address">rowScanLimits</label>
                                    <input type="text" class="form-control" id="rowScanLimits" value=""/>
                                </div>
                            </form>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-default" data-dismiss="modal">关闭
                            </button>
                            <button type="button" class="btn btn-primary" id="configs_btn">保存</button>
                        </div>
                    </div>
                </div>
            </div>

            <div class="modal fade" id="modal-container-new" role="dialog"
                 aria-labelledby="myModalLabelnew" data-backdrop="static"
                 aria-hidden="true">
                <div class="modal-dialog">
                    <div class="modal-content">
                        <div class="modal-header">
                            <button type="button" class="close" data-dismiss="modal" aria-hidden="true">
                                ×
                            </button>
                            <h4 class="modal-title" id="myModalLabelnew">新建连接</h4>
                        </div>
                        <div class="modal-body">
                            <form role="form">
                                <div class="form-group">
                                    <label for="name">名字</label>
                                    <input type="text" class="form-control" id="name" value=""/>
                                </div>
                                <div class="form-group">
                                    <label for="address">地址</label>
                                    <input type="text" class="form-control" id="address" value=""/>
                                </div>
                                <div class="form-group">
                                    <label for="port">端口</label>
                                    <input type="text" class="form-control" id="port" value="6379"/>
                                </div>
                                <div class="form-group">
                                    <label for="password">密码</label>
                                    <input type="text" class="form-control" id="password" value=""/>
                                </div>
                            </form>
                        </div>
                        <div class="modal-footer">
                            <button type="button" class="btn btn-primary" id="test_btn">测试连接
                            </button>
                            <button type="button" class="btn btn-default" data-dismiss="modal">关闭
                            </button>
                            <button type="button" class="btn btn-primary" id="new_btn">保存</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    <div class="row clearfix">
        <div class="col-md-1 column">
            <div class="list-group list-font"></div>
        </div>
        <div class="col-md-11 column" style="padding-top: 0;padding-bottom: 5px">
            <div class="row clearfix info_middle hidden">
                <div class="col-md-12 column tabbable" style="margin-top: 5px"></div>
            </div>
            <div class="row clearfix clearfix_middle">
                <div class="col-md-8 column" style="margin-top: 5px">
                    <div>
                        <nav class="navbar navbar-default" role="navigation">
                            <div class="collapse navbar-collapse" id="bs-example-navbar-collapse-3">
                                <ul class="nav navbar-nav">
                                    <li class="dropdown">
                                        <a href="#" class="dropdown-toggle" data-toggle="dropdown">数据库
                                            <strong class="caret"></strong><span class="cli_db_text"></span>
                                        </a>
                                        <ul class="dropdown-menu cli_dbs"></ul>
                                    </li>
                                    <li>
                                        <a class="menu-a refresh_db">刷新库</a>
                                    </li>
                                </ul>

                                <div class="navbar-form navbar-left">
                                    <div class="form-group">
                                        <input type="text" class="form-control match" placeholder="请输入key"/>
                                    </div>
                                    <button type="submit" class="btn btn-default scan_next">搜索key</button>
                                </div>
                                <ul class="nav navbar-nav navbar-right">
                                    <li><a class="menu-a add_key" id="modal-add"
                                           href="#modal-container-add" data-toggle="modal">添加key</a></li>
                                    <li>
                                        <a class="menu-a rename_key" id="modal-name"
                                           href="#modal-container-name" data-toggle="modal">重命名</a>
                                    </li>
                                    <li>
                                        <a class="menu-a remove_key">删除键</a>
                                    </li>
                                </ul>
                            </div>
                        </nav>
                        <div>
                            <div>
                                <p class="pull-left">下页游标
                                    <span class="iterate"></span>
                                </p>
                                <a class="btn btn-default btn-sm scan_next pull-right" role="button">翻页</a>
                            </div>
                            <ul class="nav nav-pills">
                                <select id="mySelect" multiple class="form-control"
                                        style="height: 130px;overflow: auto;float: left;margin-top: 10px"></select>
                            </ul>
                        </div>
                    </div>
                    <hr/>
                    <div>
                        <div>
                            <div class="form-group field hidden">
                                <label for="field" style="font-weight: normal;">field</label>
                                <input type="hidden" class="form-control" id="field_orignal"/>
                                <input type="text" class="form-control" id="field"/>
                            </div>
                            <div class="form-group">
                                <label for="val" style="font-weight: normal;">value
                                    <span class="hidden bit"> [二进制]</span></label>
                                <input type="hidden" class="form-control" id="val_orignal"/>
                                <textarea class="form-control" rows="3" id="val"></textarea>
                            </div>
                        </div>
                        <button type="submit" class="btn btn-default" id="update_value">保存</button>
                    </div>
                </div>

                <div class="col-md-4 column right_key" style="margin-top: 5px">
                    <div class="panel panel-default box-s">
                        <div class="panel-body" style="overflow: auto;">
                            <strong id="key_type">HASH</strong>：<span id="key" style="padding-right: 10px;"></span>
                            <br>
                            <strong>大小：</strong><span id="size"></span>
                            <br>
                            <strong>TTL：</strong><span id="ttl"></span>
                        </div>
                    </div>
                    <div style="padding-bottom: 20px" class="btn_line hidden">
                        <div class="btn-group">
                            <button class="btn btn-default refresh_key" type="button">
                                <em class="glyphicon glyphicon-refresh"></em>
                                刷新Key
                            </button>
                            <button class="btn btn-default ttl_key" type="button" id="modal-ttl"
                                    href="#modal-container-ttl" data-toggle="modal">
                                <em class="glyphicon glyphicon-time"></em>
                                TTL设置
                            </button>
                            <button class="btn btn-default insert_key" type="button" id="modal-line"
                                    href="#modal-container-line" data-toggle="modal">
                                <em class="glyphicon glyphicon-plus"></em>
                                插入行
                            </button>
                        </div>
                        <div class="btn-group remove">
                            <button class="btn btn-default del_line" type="button">
                                <em class="glyphicon glyphicon-remove"></em>
                                删除行
                            </button>
                        </div>
                    </div>
                    <div class="hidden table-responsive" id="tables_">
                        <div style="height: 205px;overflow-y: auto;margin-bottom: 13px;border: 1px solid #cccccc;border-radius: 5px;">
                            <table class="table table-condensed text-nowrap" id="tables_li"></table>
                        </div>
                        <a class="btn btn-default btn-sm list_next" role="button"
                           style="margin-bottom: 20px;">翻页</a>
                        <p style="float: right;">下页游标
                            <span class="list_iterate"></span>
                        </p>
                    </div>
                </div>

                <div class="modal fade" id="modal-container-ttl" role="dialog" aria-labelledby="myModalLabel"
                     aria-hidden="true" data-backdrop="static">
                    <div class="modal-dialog">
                        <div class="modal-content">
                            <div class="modal-header">
                                <button type="button" class="close" data-dismiss="modal" aria-hidden="true">×
                                </button>
                                <h4 class="modal-title">
                                    TTL设置
                                </h4>
                            </div>
                            <div class="modal-body">
                                <form role="form">
                                    <div class="form-group">
                                        <label for="ttl_val">新的TTL</label>
                                        <input type="number" class="form-control" id="ttl_val" value="-1"/>
                                    </div>
                                </form>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-default" data-dismiss="modal">关闭</button>
                                <button type="button" class="btn btn-primary" id="ttl_btn">保存</button>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="modal fade" id="modal-container-name" role="dialog" aria-labelledby="myModalLabel"
                     aria-hidden="true" data-backdrop="static">
                    <div class="modal-dialog">
                        <div class="modal-content">
                            <div class="modal-header">
                                <button type="button" class="close" data-dismiss="modal" aria-hidden="true">×
                                </button>
                                <h4 class="modal-title">
                                    名称
                                </h4>
                            </div>
                            <div class="modal-body">
                                <form role="form">
                                    <div class="form-group">
                                        <label for="ttl_val">新的名称</label>
                                        <input type="text" class="form-control" id="name_val" value=""/>
                                    </div>
                                </form>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-default" data-dismiss="modal">关闭</button>
                                <button type="button" class="btn btn-primary" id="name_btn">保存</button>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="modal fade" id="modal-container-add" role="dialog" aria-labelledby="myModalLabel"
                     aria-hidden="true" data-backdrop="static">
                    <div class="modal-dialog">
                        <div class="modal-content">
                            <div class="modal-header">
                                <button type="button" class="close" data-dismiss="modal" aria-hidden="true">×
                                </button>
                                <h4 class="modal-title">
                                    添加
                                </h4>
                            </div>
                            <div class="modal-body">
                                <form role="form" autocomplete="off">
                                    <div class="form-group">
                                        <label for="key_val">key</label>
                                        <input type="text" class="form-control" id="key_val" value=""/>
                                    </div>
                                    <div class="form-group">
                                        <label for="type_val">类型</label>
                                        <select id="type_val" class="form-control">
                                            <option value="string">STRING</option>
                                            <option value="bit">BIT</option>
                                            <option value="list">LIST</option>
                                            <option value="set">SET</option>
                                            <option value="zset">ZSET</option>
                                            <option value="hash">HASH</option>
                                        </select>
                                    </div>
                                    <div class="form-group hidden value_1">
                                        <label for="value_1" class="value_1_class"></label>
                                        <input type="text" class="form-control" id="value_1" value=""/>
                                    </div>
                                    <div class="form-group value_2">
                                        <label for="value_2">value</label>
                                        <input type="text" class="form-control" id="value_2" value=""/>
                                    </div>
                                </form>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-default" data-dismiss="modal">关闭</button>
                                <button type="button" class="btn btn-primary" id="add_btn">保存</button>
                            </div>
                        </div>
                    </div>
                </div>

                <div class="modal fade" id="modal-container-line" role="dialog" aria-labelledby="myModalLabel"
                     aria-hidden="true" data-backdrop="static">
                    <div class="modal-dialog">
                        <div class="modal-content">
                            <div class="modal-header">
                                <button type="button" class="close" data-dismiss="modal" aria-hidden="true">×
                                </button>
                                <h4 class="modal-title">
                                    添加
                                </h4>
                            </div>
                            <div class="modal-body">
                                <form role="form" autocomplete="off">
                                    <div class="form-group hidden value_1">
                                        <label for="value_1" class="value_1_class"></label>
                                        <input type="text" class="form-control" id="line_value_1" value=""/>
                                    </div>
                                    <div class="form-group value_2">
                                        <label for="value_2">value</label>
                                        <input type="text" class="form-control" id="line_value_2" value=""/>
                                    </div>
                                </form>
                            </div>
                            <div class="modal-footer">
                                <button type="button" class="btn btn-default" data-dismiss="modal">关闭</button>
                                <button type="button" class="btn btn-primary" id="add_line_btn">保存</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="row clearfix">
                <div class="col-md-12 column" style="margin: 0">
                    <div id="msgArea" style="padding: 5px;border:1px solid #cccccc;height: 150px;margin-bottom: 5px;text-align:start;resize: none;overflow-y: scroll"></div>
                    <input style="padding: 5px;width:100%;border:1px solid #cccccc;height: 30px;" id="userMsg" placeholder="请输入命令">
                </div>
            </div>
        </div>
    </div>
</div>

<script>
    $(document).ready(function () {
        $(".remove_key").click(function () {
            if (confirm("确定删除该键？")) {
                req('POST', 'remove_key', {
                    key: $('#mySelect option:selected').val(),
                    db: $(".cli_dbs .active").attr("data-db"),
                    id: $(".list-font .active").attr("data-id"),
                }, function (res) {
                    alert("删除成功")
                    getNewCli($(".cli_dbs .active").attr("data-db"))
                })
            } else {
                console.log('no del')
            }
        })

        r = ''
        for (i = 0; i <= 15; i++) {
            r += '<li><a class="cli_db" data-db="' + i + '">db' + i + '</a></li>'
        }
        $(".cli_dbs").html(r)

        req('GET', 'query_servers', {}, function (res) {
            html = ''
            $.each(res.servers, function (index, value) {
                html += '<a onclick="cliClick(' + value.id + ')" class="list-group-item cli_redis cli_redis_' + value.id + '" data-id="' + value.id + '" data-name="' + value.name + '"><div class="son"><ul><li onclick="infoClick(' + value.id + ')">信息</li><li onclick="delClick(' + value.id + ')">删除</li></ul></div>' + value.name + '</a>'
            });

            $("#keyScanLimits").val(res.system.keyScanLimits)
            $("#rowScanLimits").val(res.system.rowScanLimits)

            $(".list-group").html(html)
        })

        cliClick = function (res) {
            d = ".cli_redis_" + res
            req('GET', 'new_cli', {db: -1, id: res}, function () {
                $(".cli_redis").removeClass("active")
                $(d).addClass('active')
                $(".clearfix_middle").removeClass("hidden")
                $(".info_middle").addClass("hidden")
                alert("连接成功")
                link(res);
            })
        }

        delClick = function (res) {
            event.stopPropagation();
            if (confirm("确定删除连接？")) {
                req('POST', 'del_cli', {
                    id: res,
                }, function (callback) {
                    alert("删除成功")
                    window.location.reload()
                })
            } else {
                console.log('no del')
            }
        }

        infoClick = function (res) {
            event.stopPropagation();
            d = ".cli_redis_" + res
            req('GET', 'new_cli', {db: -1, id: res}, function () {
                req('GET', 'info_cli', {db: -1, id: res}, function (r) {
                    l = r.split("# ")
                    html = '<ul class="nav nav-tabs">'
                    $.each(l, function (index, value) {
                        if (value) {
                            t = value.split("\n")
                            if (index === 1) {
                                a = 'active'
                            } else {
                                a = ''
                            }
                            html += '<li class="' + a + '"><a href="#tab' + index + '" data-toggle="tab">' + t[0] + '</a></li>'
                        }
                    });
                    html = html + '</ul><div class="tab-content">'
                    $.each(l, function (index, value) {
                        if (value) {
                            if (index === 1) {
                                a = 'active'
                            } else {
                                a = ''
                            }
                            h = '<div class="tab-pane ' + a + '" id="tab' + index + '"><ul>'
                            l = value.split("\n")
                            $.each(l, function (i, v) {
                                h += '<li>' + v + '</li>'
                            });
                            html += h + '</ul></div>'
                        }
                    });
                    $(".tabbable").html(html + '</div>')
                    $(".cli_redis").removeClass("active")
                    $(d).addClass('active')
                    $(".clearfix_middle").addClass("hidden")
                    $(".info_middle").removeClass("hidden")
                })
            })
        }

        $(".cli_db").click(function () {
            t = $(this)
            db = t.attr("data-db")
            getNewCli(db)
        });

        function getNewCli(db) {
            req('GET', 'new_cli', {db: db, id: $(".list-font .active").attr("data-id")}, function (res) {
                $(".cli_db_text").text(" db" + db)
                $(".cli_db").removeClass("active")
                t.addClass("active")
                $(".scan_next").attr("data-v", res.iterate)
                $(".iterate").text(res.iterate)

                html = ''
                $.each(res.keys, function (index, value) {
                    html += ' <option class="keys_padding" title="' + value + '">' + value + '</option>'
                });
                $("#mySelect").html(html)
            })
        }

        $(".scan_next").click(function () {
            req('GET', 'scan_page', {
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
                iterate: $(".scan_next").attr("data-v"),
                match: $(".match").val()
            }, function (res) {
				$("#tables_").addClass("hidden")
                $(".field").addClass("hidden")
                $(".btn_line").removeClass("hidden")
                $("#update_value").removeClass("hidden")
                $(".remove").addClass("hidden")
                $(".right_key").addClass("hidden")
                $(".insert_key").addClass("hidden")
                $("#tables_li").html('')
                $("#field").attr("data-index", null)
                $("#field").val('')
                $("#val").val('')
                $("#val_orignal").val('')
                $("#field_orignal").val('')

                $(".scan_next").attr("data-v", res.iterate)
                $(".iterate").text(res.iterate)
                html = ''
                $.each(res.keys, function (index, value) {
                    html += ' <option class="keys_padding" title="' + value + '">' + value + '</option>'
                });
                $("#mySelect").html(html)
            })
        });

        $('.refresh_db').click(function () {
			$("#tables_").addClass("hidden")
			$(".field").addClass("hidden")
			$(".btn_line").removeClass("hidden")
			$("#update_value").removeClass("hidden")
			$(".remove").addClass("hidden")
			$(".right_key").addClass("hidden")
			$(".insert_key").addClass("hidden")
			$("#tables_li").html('')
			$("#field").attr("data-index", null)
			$("#field").val('')
			$("#val").val('')
			$("#val_orignal").val('')
			$("#field_orignal").val('')

            $(".match").val("")
            getNewCli($(".cli_dbs .active").attr("data-db"))
        });

        $('.refresh_key').click(function () {
            getData($('#mySelect option:selected').val(), -1)
        });

        $('#mySelect').click(function () {
            if ($('#mySelect option:selected').val()) {
                getData($('#mySelect option:selected').val(), -1)
            }
        });

        $(".list_next").click(function () {
            getData($('#mySelect option:selected').val(), 0)
        });

        function getData(key, iterate) {
            req('GET', 'select_key', {
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
                cursor: iterate ? 0 : $(".list_next").attr("data-cursor"),
                match: '',
                key: key
            }, function (res) {
                if (res.bit) {
                    $(".bit").removeClass("hidden")
                } else {
                    $(".bit").addClass("hidden")
                }
                key_type = res.t
                $("#tables_").addClass("hidden")
                $(".field").addClass("hidden")
                $(".btn_line").removeClass("hidden")
                $("#update_value").removeClass("hidden")
                $(".right_key").removeClass("hidden")
                $(".remove").addClass("hidden")
                $(".insert_key").addClass("hidden")
                $("#tables_li").html('')
                $("#field").attr("data-index", null)
                $("#field").val('')
                $("#val").val('')
                $("#val_orignal").val('')
                $("#field_orignal").val('')

                $("#key_type").text(res.t.toUpperCase())
                $("#ttl").text(res.ttl)
                $("#size").text(res.size)
                $("#key").text(key)
                html = ""
                if (key_type !== "string") {
                    html = '<thead><tr><th>row</th>'
                    if (res.val.length > 0) {
                        if (key_type == "hash") {
                            $(".field").removeClass("hidden")
                            html += '<th>value</th><th>key</th>'
                        }
                        if (key_type == "zset") {
                            $(".field").removeClass("hidden")
                            html += '<th>value</th><th>score</th>'
                        }
                        if (key_type == "list" || key_type == "set") {
                            html += '<th>value</th>'
                        }
                        html += '</tr><tbody>'
                        $.each(res.val, function (index, value) {
                            v = ''
                            if (key_type == "hash") {
                                v = value.field
                            } else {
                                v = value.score
                            }
                            d = JSON.stringify(value)
                            html += '<tr onclick="lineClick(' + index + ')" class="lineClick lineClick_' + index + '" data-line_v=' + d + '><td>' + (index + 1) + '</td><td>' + value.val + (v ? '</td><td>' + v + '</td>' : '') + '</tr>'
                        })
                    }
                    html += '</tbody></thead>'
                    $("#tables_").removeClass("hidden")
                    $(".remove").removeClass("hidden")
                    $(".insert_key").removeClass("hidden")
                    $("#tables_li").html(html)
                    $(".list_next").attr("data-cursor", res.iterate)
                    $(".list_iterate").text(res.iterate)
                } else {
                    $("#val").val(res.val)
                    $("#val_orignal").val(res.val)
                    b = getByteLen(res.val)
                    console.log(b)
                    if (res.bit) {
                        $("#field_orignal").val("bit")
                        $("#update_value").addClass("hidden")
                    }
                }
            })
        }

        lineClick = function (res) {
            $(".lineClick").removeClass("current")
            d = ".lineClick_" + res
            $(d).addClass('current')
            r = $(".current").attr("data-line_v")
            r = JSON.parse(r)
            $("#val").val(r.val)
            $("#val_orignal").val(r.val)
            $("#field").attr("data-index", res)
            if (r.score || r.field) {
                $(".field").removeClass("hidden")
                $("#field").val(r.score ? r.score : r.field)
                $("#field_orignal").val(r.score ? r.score : r.field)
            }
        }

        $("#update_value").click(function () {
            req('POST', 'update_value', {
                key: $('#mySelect option:selected').val(),
                field: $("#field").val(),
                fieldOrignal: $("#field_orignal").val(),
                val: $("#val").val(),
                valOrignal: $("#val_orignal").val(),
                index: $("#field").attr("data-index"),
                type: $("#key_type").text().toLowerCase(),
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
            }, function (res) {
                alert("更新成功")
                getData($('#mySelect option:selected').val(), -1)
            })
        });

        $(".del_line").click(function () {
            if (confirm("确定删除？")) {
                req('POST', 'del_line', {
                    key: $('#mySelect option:selected').val(),
                    field: $("#field").val(),
                    val: $("#val").val(),
                    type: $("#key_type").text().toLowerCase(),
                    db: $(".cli_dbs .active").attr("data-db"),
                    id: $(".list-font .active").attr("data-id"),
                }, function (res) {
                    alert("删除行成功")
                    getData($('#mySelect option:selected').val(), -1)
                })
            } else {
                console.log('no del')
            }
        });

        $("#ttl_btn").click(function () {
            req('POST', 'ttl_val', {
                key: $('#mySelect option:selected').val(),
                ttl: $('#ttl_val').val(),
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
            }, function (callback) {
                alert("更新成功")
                $("#modal-container-ttl").modal('hide')
                getData($('#mySelect option:selected').val(), -1)
            })
        });

        $("#name_btn").click(function () {
            req('POST', 'rename', {
                key: $('#mySelect option:selected').val(),
                name: $('#name_val').val(),
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
            }, function (callback) {
                alert("更新成功")
                $("#modal-container-name").modal('hide')
                getNewCli($(".cli_dbs .active").attr("data-db"))
            })
        });

        $("#configs_btn").click(function () {
            var data = {
                connectionTimeout: 300,
                executionTimeout: 10,
                delRowLimits: 99,
                keyScanLimits: parseInt($('#keyScanLimits').val()),
                rowScanLimits: parseInt($('#rowScanLimits').val())
            }
            req('POST', 'update_system_configs', {
                "data": JSON.stringify(data)
            }, function () {
                alert('设置成功')
                window.location.reload()
            })
        });

        $("#type_val").change(function () {
            $(".value_2").removeClass("hidden")
            type = $('#type_val option:selected').val()
            if (type == 'zset' || type == 'hash' || type == 'bit') {
                $(".value_1").removeClass("hidden")
                if (type == 'zset') {
                    $(".value_1_class").text('score')
                } else if (type == 'hash') {
                    $(".value_1_class").text('field')
                } else {
                    $(".value_1_class").text('offset')
                }
            } else {
                $(".value_1").addClass("hidden")
            }
        });

        $("#add_btn").click(function () {
            req('POST', 'add_key', {
                type: $('#type_val option:selected').val(),
                key: $('#key_val').val(),
                val_1: $('#value_1').val(),
                val_2: $('#value_2').val(),
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
            }, function (callback) {
                alert("添加成功")
                $("#modal-container-add").modal('hide')
                getNewCli($(".cli_dbs .active").attr("data-db"))
            })
        });

        $(".insert_key").click(function () {
            $(".value_2").removeClass("hidden")
            type = $("#key_type").text().toLowerCase()
            if (type == 'zset' || type == 'hash') {
                $(".value_1").removeClass("hidden")
                if (type == 'zset') {
                    $(".value_1_class").text('score')
                } else if (type == 'hash') {
                    $(".value_1_class").text('field')
                } else {
                    $(".value_1_class").text('offset')
                }
            } else {
                $(".value_1").addClass("hidden")
            }
        });

        $("#add_line_btn").click(function () {
            req('POST', 'add_key', {
                type: $("#key_type").text().toLowerCase(),
                key: $('#mySelect option:selected').val(),
                val_1: $('#line_value_1').val(),
                val_2: $('#line_value_2').val(),
                db: $(".cli_dbs .active").attr("data-db"),
                id: $(".list-font .active").attr("data-id"),
            }, function (callback) {
                alert("添加成功")
                $("#modal-container-line").modal('hide')
                getData($('#mySelect option:selected').val(), -1)
            })
        });

        $("#test_btn").click(function () {
            req('GET', 'test_btn', {
                name: $('#name').val(),
                address: $('#address').val(),
                port: $('#port').val(),
                password: $('#password').val(),
            }, function (callback) {
                alert("连接成功")
            })
        });

        $("#new_btn").click(function () {
            req('POST', 'save_cli', {
                name: $('#name').val(),
                address: $('#address').val(),
                port: $('#port').val(),
                password: $('#password').val(),
            }, function (callback) {
                alert("保存成功")
                window.location.reload()
            })
        });
    });
</script>

<script>
    function req(type, url, data, callback) {
        $.ajax({
            type: type,
            url: 'http://127.0.0.1:9681/' + url,
            data: data,
            dataType: "json",
            contentType: 'application/x-www-form-urlencoded',
            success: function (response) {
                console.log(response)
                if (response.Code) {
                    callback(response.Data)
                    return true
                } else {
                    alert(response.Reason)
                }
            },
            error: function (xhr) {
                alert(xhr)
            }
        });
    }

    function getByteLen(val) {
        var len = 0;
        for (var i = 0; i < val.length; i++) {
            if (val[i].match(/[^\x00-\xff]/ig) != null) {  //全角
                len += 2;
            } else
                len += 1;
        }
        return len;
    }

</script>
<script>
    var ws;
    var select_db = "0";

    function link(id) {
        ws = new WebSocket("ws://127.0.0.1:9681/ws?id=" + id);//连接服务器
        ws.onopen = function (event) {
            // $("#msgArea").html("");
            ws.send("ping");
            $("#userMsg").val("ping")
        };
        ws.onmessage = function (event) {
            var date = new Date();
            var msg = "<p style='margin: 0'>" + date.toLocaleString() + " 数据库：" + select_db + " 命令：" + $("#userMsg").val() + "</p>" + "<p>" + "响应：<br/>" + event.data + "</p>";
            $("#msgArea").append(msg);
            $("#userMsg").val("");
            document.getElementById("msgArea").scrollTop = document.getElementById("msgArea").scrollHeight;
        }
        ws.onclose = function (event) {
            alert("已经与服务器断开连接\r\n当前连接状态：" + this.readyState);
        };
        ws.onerror = function (event) {
            alert("WebSocket异常！");
        };
    }

    setInterval(function () {
        if ($(".list-font .active").attr("data-id")) {
            ws.send("");
            req('GET', 'new_cli', {
                db: $(".cli_dbs .active").attr("data-db") ? $(".cli_dbs .active").attr("data-db") : -1,
                id: $(".list-font .active").attr("data-id")
            }, function () {
            })
        }
    }, 20000);

    $(document).keyup(function (event) {
        if (event.keyCode == 13 && ws) {
            var msg = $("#userMsg").val();
            if (msg.indexOf("select") == 0) {
                str = msg.replace("select", "");
                select_db = $.trim(str) ? $.trim(str) : select_db
            }
            if ($.trim(msg)) {
                ws.send($.trim(msg));
            }
        }
    });
</script>
<style>
    .container {
        width: auto;
        margin: 0 0;
    }

    .column {
        background-color: #FFFFFF;
        border: 1px solid #DDDDDD;
        border-radius: 4px 4px 4px 4px;
        margin: 15px 0;
        padding: 39px 19px 24px;
        position: relative;
    }

    body.devpreview {
        margin: 2px;
    }

    .devpreview .sidebar-nav {
        left: -200px;
        -webkit-transition: all 0ms ease;
        -moz-transition: all 0ms ease;
        -ms-transition: all 0ms ease;
        -o-transition: all 0ms ease;
        transition: all 0ms ease;
    }

    .box-s {
        box-shadow: none;
    }

    .list-font {
        font-size: 12px;
    }

    .cli_dbs li {
        cursor: pointer;
    }

    .keys_padding {
        padding: 7px 5px;
        font-size: 15px;
        cursor: pointer;
    }

    #tables_li tbody tr, .menu-a {
        cursor: pointer;
    }

    .current {
        background: #c8c8c8;
    }

    .son {
        display: none;
    }

    .son ul li {
        display: block;
        list-style: none;
        background: #f8f8f8;
        width: 60px;
        height: 30px;
        border: 1px solid #dddddd;
        margin: 5px 0;
        padding: 0;
        text-align: center;
        line-height: 30px;
        color: #333;
        border-radius: 3px;
    }

    .cli_redis {
        cursor: pointer;
    }

    .cli_redis:hover .son {
        display: block;
        position: absolute;
        left: 80px;
        z-index: 999;
    }

    .tabbable ul {
        padding-left: 0;
        margin-top: 10px;
    }

    .tabbable ul li {
        list-style: none;
    }

    .tab-pane ul {
        height: 540px;
    }

    .tab-pane ul li {
        float: left;
        padding: 3px;
        width: 600px;
    }
</style>
</body>
</html>`

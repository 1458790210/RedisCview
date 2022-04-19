package main

import (
	"RedisCview/lib"
	"bufio"
	"bytes"
	"embed"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/zserge/lorca"
	"io/fs"
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

//go:embed frontend/*
var embededFiles embed.FS

func getFileSystem(useOS bool) http.FileSystem {
	if useOS {
		return http.FS(os.DirFS("static"))
	}

	fsys, err := fs.Sub(embededFiles, "frontend")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}

func main() {
	http.Handle("/", http.FileServer(getFileSystem(false)))

	//http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	//	w.Header().Set("Content-Type", "text/html")
	//	fmt.Fprint(w, AddForm)
	//	return
	//})

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

const AddForm = ""

package main

// 健作くん

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"text/template"
	"time"
)

const (
	TIMEOUT_HANDLER_SEC = 25 * time.Second
	TIMEOUT_READ_SEC    = 30 * time.Second
	TIMEOUT_WRITE_SEC   = 30 * time.Second
	TIMEOUT_MESSAGE     = "テンポってます。"
	MAX_HEADER_SIZE     = 1024 * 100
	DBConnSize          = 2
	DBIdleSize          = 2
	DBUser              = "kensakukun"
	DBName              = "kensakukun"
	DBPass              = "hagehagehagering"
	DBHost              = "127.0.0.1:4223"
	DBTable             = "kensakukun"
)

type OtegaruHandle struct {
	fs http.Handler
}

type OtegaruItem struct {
	Uid  int64
	Id   string
	Pass string
}

type OtegaruPage struct {
	Hit   int64
	Msg   string
	Items []OtegaruItem
}

type Output struct {
	Buf bytes.Buffer
}

var HtmlTmpl = template.Must(template.ParseFiles("template/search.html"))
var Stdlog = log.New(os.Stdout, "", 0)

func main() {
	cpus := runtime.NumCPU()
	runtime.GOMAXPROCS(cpus * 4)
	server := &http.Server{
		Addr: ":80",
		Handler: http.TimeoutHandler(&OtegaruHandle{
			fs: http.FileServer(http.Dir("public_html")),
		}, TIMEOUT_HANDLER_SEC, TIMEOUT_MESSAGE),
		ReadTimeout:    TIMEOUT_READ_SEC,
		WriteTimeout:   TIMEOUT_WRITE_SEC,
		MaxHeaderBytes: MAX_HEADER_SIZE,
	}
	log.Printf("listen start %s\n", server.Addr)
	// サーバ起動
	log.Fatal(server.ListenAndServe())
}

func (lh *OtegaruHandle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer dispose(w, r)

	var code int
	var size int
	if r.URL.Path == "/" {
		var buf io.Reader
		var out io.Writer
		code, size, buf = analyze(r)
		ae := r.Header.Get("Accept-Encoding")
		if strings.Contains(ae, "gzip") {
			// gzip圧縮
			w.Header().Set("Content-Encoding", "gzip")
			gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
			defer gz.Close()
			out = gz
		} else {
			// 生データ
			out = w
		}
		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		io.Copy(out, buf)
	} else {
		// 後はファイルサーバーさんに任せる
		wrw := &WrapperResponseWriter{W: w}
		lh.fs.ServeHTTP(wrw, r)
		code = wrw.code
		size = wrw.size
	}
	putlog(r, code, size)
}

func analyze(r *http.Request) (int, int, io.Reader) {
	code := 200
	size := 0
	out := &bytes.Buffer{}
	if r.URL.RawQuery != "" {
		HtmlTmpl.Execute(out, search(r))
	} else {
		HtmlTmpl.Execute(out, OtegaruPage{})
	}
	size = out.Len()
	return code, size, out
}

func search(r *http.Request) OtegaruPage {
	page := OtegaruPage{}

	sl := []string{"uid", "id", "pass"}
	ql := []string{}

	q := r.URL.Query()
	id := q.Get("id")
	pass := q.Get("pass")
	if id != "" {
		ql = append(ql, fmt.Sprintf("id like '%s%%'", sqlEscape(id)))
	}
	if pass != "" {
		ql = append(ql, fmt.Sprintf("pass like '%s%%'", sqlEscape(pass)))
	}
	if id == "" && pass == "" {
		return page
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 0, 100",
		strings.Join(sl, ", "),
		DBTable,
		strings.Join(ql, " AND "))
	con, err := connect()
	if err != nil {
		page.Msg = "検索エラー1"
		return page
	}
	defer con.Close()

	// 取得
	rows, err := con.Query(query)
	if err != nil {
		page.Msg = "検索エラー2"
		return page
	}
	defer rows.Close()

	for rows.Next() {
		var it OtegaruItem
		err = rows.Scan(&it.Uid, &it.Id, &it.Pass)
		it.Id = template.HTMLEscapeString(it.Id)
		it.Pass = template.HTMLEscapeString(it.Pass)
		if err != nil {
			page.Msg = "検索エラー3"
			return page
		}
		page.Items = append(page.Items, it)
	}
	if err = rows.Err(); err != nil {
		page.Msg = "検索エラー4"
		return page
	}

	page.Hit = selectFoundRows(con)
	if id != "" && pass != "" {
		page.Hit++
		page.Msg = `<strong>こういう怪しいサイトでIDとPASSの両方を入力してはいけません。</strong>`
		page.Items = append(page.Items, OtegaruItem{
			Uid:  99999999,
			Id:   template.HTMLEscapeString(id),
			Pass: template.HTMLEscapeString(pass),
		})
	}
	return page
}

func putlog(r *http.Request, code, size int) {
	rh, _, _ := net.SplitHostPort(r.RemoteAddr)
	date := time.Now().Format("02/Jan/2006:15:04:05 -0700")
	p := r.URL.Path
	if r.URL.RawQuery != "" {
		p += "?" + r.URL.RawQuery
	}
	Stdlog.Printf(`%s - - [%s] "%s %s %s" %d %d`, rh, date, r.Method, p, r.Proto, code, size)
}

func dispose(w http.ResponseWriter, r *http.Request) {
	if err := recover(); err != nil {
		// 500を返しておく
		code := http.StatusInternalServerError
		w.WriteHeader(code)
		// ログ出力
		putlog(r, code, 0)
	}
}

type WrapperResponseWriter struct {
	W    http.ResponseWriter
	code int
	size int
}

func (wrw *WrapperResponseWriter) Header() http.Header {
	return wrw.W.Header()
}

func (wrw *WrapperResponseWriter) Write(buf []byte) (size int, err error) {
	size, err = wrw.W.Write(buf)
	wrw.size += size
	return
}

func (wrw *WrapperResponseWriter) WriteHeader(code int) {
	wrw.W.WriteHeader(code)
	wrw.code = code
}

func selectFoundRows(con *sql.DB) int64 {
	var searchmax int64
	result, err := con.Query("SELECT FOUND_ROWS()")
	if err == nil {
		result.Next()
		err = result.Scan(&searchmax)
		if err != nil {
			searchmax = 0
		}
		result.Close()
	}
	return searchmax
}

func sqlEscape(s string) string {
	return strings.Replace(s, "'", "''", -1)
}

func connect() (*sql.DB, error) {
	con, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", DBUser, DBPass, DBHost, DBName))
	if err == nil {
		con.SetMaxOpenConns(DBConnSize)
		con.SetMaxIdleConns(DBIdleSize)
	}
	return con, err
}

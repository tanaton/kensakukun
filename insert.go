package main

// 健作くんの前処理
/*
CREATE TABLE kensakukun(
uid INT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
id VARCHAR(32),
pass VARCHAR(32),
INDEX id_index (id(5)),
INDEX pass_index (pass(5))
) DEFAULT CHARSET utf8 ENGINE = innodb;

SET @@GLOBAL.sql_mode='NO_ENGINE_SUBSTITUTION,STRICT_TRANS_TABLES,NO_BACKSLASH_ESCAPES';
*/

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"strings"
)

const (
	FileName   = "10-million-combos.txt"
	DBConnSize = 2
	DBIdleSize = 2
	DBUser     = "kensakukun"
	DBName     = "kensakukun"
	DBPass     = "hagehagehagering"
	DBHost     = "54.65.61.223:4223"
	DBTable    = "kensakukun"
)

func main() {
	fp, err := os.Open(FileName)
	if err != nil {
		return
	}
	defer fp.Close()
	scanner := bufio.NewScanner(fp)
	list := []string{}
	var con *sql.DB
	for scanner.Scan() {
		it := scanner.Text()
		l := strings.Split(it, "\t")
		if len(l) <= 1 {
			continue
		}
		query := fmt.Sprintf(
			"('%s','%s')",
			sqlEscape(l[0]),
			sqlEscape(l[1]))
		list = append(list, query)
		if len(list) >= 128 {
			con = insert(con, list)
			list = []string{}
		}
	}
	con = insert(con, list)
}

func insert(con *sql.DB, list []string) *sql.DB {
	if con == nil {
		con, _ = connect()
	}
	if con != nil {
		query := fmt.Sprintf("INSERT INTO %s (id,pass) VALUES%s",
			DBTable,
			strings.Join(list, ", "))
		_, err := con.Exec(query)
		if err != nil {
			con.Close()
			con = nil
		} else {
			fmt.Printf("insert:%d\n", len(list))
		}
	}
	return con
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

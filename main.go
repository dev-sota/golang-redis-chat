package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/soveran/redisurl"
)

func main() {
	// Redisに接続する
	conn, err := redisurl.ConnectToURL("redis://localhost:6379")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer conn.Close()

	// ユーザーの作成
	userName := os.Args[1]
	userKey := "online." + userName

	// TTL
	val, err := conn.Do("SET", userKey, userName, "NX", "EX", "120")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if val == nil {
		fmt.Println("User is already online")
		os.Exit(1)
	}

	// ユーザーリストの設定
	val, err = conn.Do("SADD", "users", userName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if val == nil {
		fmt.Println("User is already online")
		os.Exit(1)
	}

	tickerChan := time.NewTicker(time.Second * 60).C

	// メッセージの送受信
	subChan := make(chan string)
	go func() {
		subConn, err := redisurl.ConnectToURL("redis://localhost:6379")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer subConn.Close()

		// サブスクライブするチャンネルを設定
		psc := redis.PubSubConn{Conn: subConn}
		_ = psc.Subscribe("messages")
		for {
			switch v := psc.Receive().(type) {
			case redis.Message:
				subChan <- string(v.Data)
			case redis.Subscription:
				break
			case error:
				return
			}
		}
	}()

	// ターミナルからコマンドを読み込む
	sayChan := make(chan string)
	go func() {
		prompt := userName + ">"
		bio := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(prompt)
			line, _, err := bio.ReadLine()
			if err != nil {
				fmt.Println(err)
				sayChan <- "/exit"
				return
			}
			sayChan <- string(line)
		}
	}()

	_, _ = conn.Do("PUBLISH", "messages", userName+" has joined")

	// chatExitフラグがtrueに設定されている時だけチャットから退出
	chatExit := false

	for !chatExit {
		select {
		case msg := <-subChan:
			fmt.Println(msg)
		case <-tickerChan:
			val, err = conn.Do("SET", userKey, userName, "XX", "EX", "120")
			if err != nil || val == nil {
				fmt.Println("Heartbeat set failed")
				chatExit = true
			}
		case line := <-sayChan:
			if line == "/exit" {
				chatExit = true
			} else if line == "/who" {
				names, _ := redis.Strings(conn.Do("SMEMBERS", "users"))
				for _, name := range names {
					fmt.Println(name)
				}
			} else {
				_, _ = conn.Do("PUBLISH", "messages", userName+":"+line)
			}
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	// チャットからの退出
	_, _ = conn.Do("DEL", userKey)
	_, _ = conn.Do("SREM", "users", userName)
	_, _ = conn.Do("PUBLISH", "messages", userName+" has left")
}

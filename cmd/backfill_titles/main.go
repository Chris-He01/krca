// backfill_titles 重建历史 session 标题。
//
// 对于标题为空、乱码（含 \uFFFD 替换字符）或过短（< 3 字）的 session，
// 从首条 user message 的内容重新截断生成标题。
//
// 用法（SQLite）：
//   go run ./cmd/backfill_titles -db /path/to/knsight.db [-dry-run]
//
// 用法（Redis）：
//   go run ./cmd/backfill_titles -redis-resource <resource_name> -redis-prefix <prefix> [-dry-run]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"knsight-go/internal/hub/store"
)

func main() {
	dbPath := flag.String("db", "", "SQLite DB 路径")
	redisResource := flag.String("redis-resource", "", "Redis resource_name（kedis）")
	redisPrefix := flag.String("redis-prefix", "", "Redis key prefix（如 prod）")
	dryRun := flag.Bool("dry-run", false, "只打印不实际写入")
	flag.Parse()

	ctx := context.Background()

	var s store.SessionStore
	if *dbPath != "" {
		db, err := store.NewDB(*dbPath)
		if err != nil {
			log.Fatalf("open sqlite: %v", err)
		}
		defer db.Close()
		s = store.NewSQLiteStore(db)
		log.Printf("store: sqlite %s", *dbPath)
	} else if *redisResource != "" {
		rs, err := store.NewRedisStore(*redisResource, *redisPrefix)
		if err != nil {
			log.Fatalf("open redis: %v", err)
		}
		defer rs.Close()
		s = rs
		log.Printf("store: redis resource=%s prefix=%s", *redisResource, *redisPrefix)
	} else {
		log.Fatalf("必须指定 -db 或 -redis-resource")
	}

	sessions, err := s.ListSessions(ctx, "", 100000, 0, "", nil)
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	log.Printf("共 %d 个 session，开始检查标题…", len(sessions))

	fixed, skipped := 0, 0
	for _, sess := range sessions {
		if !needsBackfill(sess.Title) {
			skipped++
			continue
		}

		msgs, err := s.GetMessages(ctx, sess.ID)
		if err != nil || len(msgs) == 0 {
			log.Printf("[%s] 无消息，跳过", sess.ID[:min(12, len(sess.ID))])
			skipped++
			continue
		}

		firstUser := ""
		for _, m := range msgs {
			if m.Role == "user" {
				firstUser = m.Content
				break
			}
		}
		if firstUser == "" {
			skipped++
			continue
		}

		newTitle := truncateRunes(firstUser, 50)
		if newTitle == sess.Title {
			skipped++
			continue
		}

		fmt.Printf("[%s] %q → %q\n", sess.ID[:min(12, len(sess.ID))], sess.Title, newTitle)
		fixed++

		if !*dryRun {
			sess.Title = newTitle
			if err := s.UpdateSession(ctx, sess); err != nil {
				log.Printf("[%s] update error: %v", sess.ID[:min(12, len(sess.ID))], err)
			}
		}
	}

	if *dryRun {
		fmt.Printf("\n[dry-run] 需修复 %d 个，跳过 %d 个（未写入）\n", fixed, skipped)
	} else {
		fmt.Printf("\n修复 %d 个，跳过 %d 个\n", fixed, skipped)
	}
}

// needsBackfill 返回 true 表示标题需要重建：
//   - 空标题
//   - 含 UTF-8 替换字符 \uFFFD（乱码）
//   - 有效 rune 数 < 3（过短，可能是截断错误）
func needsBackfill(title string) bool {
	if title == "" {
		return true
	}
	if strings.ContainsRune(title, utf8.RuneError) {
		return true
	}
	if utf8.RuneCountInString(title) < 3 {
		return true
	}
	return false
}

// truncateRunes 按 Unicode rune 截断，避免在多字节字符中间切断。
// 换行符替换为空格，保持单行。
func truncateRunes(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(rs[:maxRunes])
	}
	return string(rs[:maxRunes-3]) + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

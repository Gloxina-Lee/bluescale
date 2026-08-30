package main

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"bluescale/internal/app"
	"golang.org/x/term"
)

//go:embed all:web/dist
var webFiles embed.FS

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "admin":
			return runAdminCommand(args[1:], stdin, stdout, stderr)
		case "help", "-h", "--help":
			printUsage(stdout)
			return 0
		default:
			fmt.Fprintf(stderr, "未知命令：%s\n\n", args[0])
			printUsage(stderr)
			return 2
		}
	}
	if err := serve(); err != nil {
		fmt.Fprintf(stderr, "BlueScale 启动失败：%v\n", err)
		return 1
	}
	return 0
}

func serve() error {
	addr := envOr("BLUESCALE_ADDR", ":8080")
	dataDir := envOr("BLUESCALE_DATA_DIR", "data")

	frontend, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		return err
	}

	application, err := app.New(app.Config{DataDir: dataDir, Frontend: frontend})
	if err != nil {
		return err
	}
	defer application.Close()

	server := &http.Server{
		Addr:              addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	log.Printf("BlueScale is listening on %s", displayURL(addr))
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runAdminCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printAdminUsage(stderr)
		return 2
	}
	switch args[0] {
	case "show-username":
		flags := newFlagSet("admin show-username", stderr)
		dataDir := flags.String("data-dir", envOr("BLUESCALE_DATA_DIR", "data"), "BlueScale 数据目录")
		flags.Usage = func() { fmt.Fprintln(flags.Output(), "用法：bluescale admin show-username [--data-dir DIR]") }
		if code := parseFlags(flags, args[1:]); code >= 0 {
			return code
		}
		if flags.NArg() != 0 {
			flags.Usage()
			return 2
		}
		recovery, err := app.OpenAdministratorRecovery(*dataDir)
		if err != nil {
			return reportAdminError(stderr, err)
		}
		defer recovery.Close()
		username, err := recovery.Username()
		if err != nil {
			return reportAdminError(stderr, err)
		}
		fmt.Fprintf(stdout, "数据库：%q\n管理员用户名：%q\n", recovery.DatabasePath(), username)
		return 0

	case "reset-username":
		flags := newFlagSet("admin reset-username", stderr)
		dataDir := flags.String("data-dir", envOr("BLUESCALE_DATA_DIR", "data"), "BlueScale 数据目录")
		yes := flags.Bool("yes", false, "跳过交互确认")
		flags.Usage = func() {
			fmt.Fprintln(flags.Output(), "用法：bluescale admin reset-username [--data-dir DIR] [--yes] NEW_USERNAME")
		}
		if code := parseFlags(flags, args[1:]); code >= 0 {
			return code
		}
		if flags.NArg() != 1 {
			flags.Usage()
			return 2
		}
		recovery, err := app.OpenAdministratorRecovery(*dataDir)
		if err != nil {
			return reportAdminError(stderr, err)
		}
		defer recovery.Close()
		currentUsername, err := recovery.Username()
		if err != nil {
			return reportAdminError(stderr, err)
		}
		confirmed, err := confirmRecovery(stdin, stderr, *yes, recovery.DatabasePath(), currentUsername, fmt.Sprintf("将用户名修改为 %q，并撤销全部登录会话", flags.Arg(0)))
		if err != nil {
			return reportAdminError(stderr, err)
		}
		if !confirmed {
			fmt.Fprintln(stdout, "已取消。")
			return 0
		}
		username, err := recovery.ResetUsername(flags.Arg(0))
		if err != nil {
			return reportAdminError(stderr, err)
		}
		fmt.Fprintf(stdout, "管理员用户名已重置为 %q；全部登录会话已撤销。\n", username)
		return 0

	case "reset-password":
		flags := newFlagSet("admin reset-password", stderr)
		dataDir := flags.String("data-dir", envOr("BLUESCALE_DATA_DIR", "data"), "BlueScale 数据目录")
		yes := flags.Bool("yes", false, "跳过交互确认")
		passwordStdin := flags.Bool("password-stdin", false, "从标准输入读取一行密码")
		revokeAPITokens := flags.Bool("revoke-api-tokens", false, "同时撤销全部 API Token")
		flags.Usage = func() {
			fmt.Fprintln(flags.Output(), "用法：bluescale admin reset-password [--data-dir DIR] [--yes] [--password-stdin] [--revoke-api-tokens]")
		}
		if code := parseFlags(flags, args[1:]); code >= 0 {
			return code
		}
		if flags.NArg() != 0 {
			flags.Usage()
			return 2
		}
		if *passwordStdin && !*yes {
			return reportAdminError(stderr, errors.New("使用 --password-stdin 时必须同时指定 --yes，避免确认输入与密码混淆"))
		}
		recovery, err := app.OpenAdministratorRecovery(*dataDir)
		if err != nil {
			return reportAdminError(stderr, err)
		}
		defer recovery.Close()
		currentUsername, err := recovery.Username()
		if err != nil {
			return reportAdminError(stderr, err)
		}
		action := "将重置管理员密码，并撤销全部登录会话"
		if *revokeAPITokens {
			action += "及 API Token"
		}
		confirmed, err := confirmRecovery(stdin, stderr, *yes, recovery.DatabasePath(), currentUsername, action)
		if err != nil {
			return reportAdminError(stderr, err)
		}
		if !confirmed {
			fmt.Fprintln(stdout, "已取消。")
			return 0
		}
		password, err := readNewPassword(stdin, stderr, *passwordStdin)
		if err != nil {
			return reportAdminError(stderr, err)
		}
		defer wipeBytes(password)
		if err := recovery.ResetPassword(password, *revokeAPITokens); err != nil {
			return reportAdminError(stderr, err)
		}
		fmt.Fprintln(stdout, "管理员密码已重置；全部登录会话已撤销。")
		if *revokeAPITokens {
			fmt.Fprintln(stdout, "全部 API Token 已撤销。")
		} else {
			fmt.Fprintln(stdout, "API Token 保持有效；如怀疑凭据泄露，请使用 --revoke-api-tokens 再次重置。")
		}
		return 0

	case "help", "-h", "--help":
		printAdminUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "未知管理员命令：%s\n\n", args[0])
		printAdminUsage(stderr)
		return 2
	}
}

func newFlagSet(name string, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(output)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) int {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	return -1
}

func confirmRecovery(stdin io.Reader, stderr io.Writer, yes bool, databasePath, username, action string) (bool, error) {
	fmt.Fprintf(stderr, "数据库：%q\n当前管理员：%q\n操作：%s\n", databasePath, username, action)
	if yes {
		return true, nil
	}
	fmt.Fprint(stderr, "输入 yes 确认：")
	line, err := bufio.NewReader(io.LimitReader(stdin, 32)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("读取确认输入：%w", err)
	}
	return strings.TrimSpace(line) == "yes", nil
}

func readNewPassword(stdin io.Reader, stderr io.Writer, fromStdin bool) ([]byte, error) {
	if fromStdin {
		password, err := io.ReadAll(io.LimitReader(stdin, 74))
		if err != nil {
			return nil, fmt.Errorf("读取新密码：%w", err)
		}
		password = bytes.TrimSuffix(password, []byte{'\n'})
		password = bytes.TrimSuffix(password, []byte{'\r'})
		if bytes.ContainsAny(password, "\r\n") {
			wipeBytes(password)
			return nil, errors.New("--password-stdin 只能读取一行密码")
		}
		return password, nil
	}
	input, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return nil, errors.New("当前标准输入不是终端；自动化环境请使用 --password-stdin --yes")
	}
	fmt.Fprint(stderr, "输入新密码：")
	first, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return nil, fmt.Errorf("读取新密码：%w", err)
	}
	fmt.Fprint(stderr, "再次输入新密码：")
	second, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		wipeBytes(first)
		return nil, fmt.Errorf("读取确认密码：%w", err)
	}
	defer wipeBytes(second)
	if subtle.ConstantTimeCompare(first, second) != 1 {
		wipeBytes(first)
		return nil, errors.New("两次输入的密码不一致")
	}
	return first, nil
}

func wipeBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func reportAdminError(stderr io.Writer, err error) int {
	if errors.Is(err, app.ErrInstanceRunning) {
		fmt.Fprintln(stderr, "恢复操作被拒绝：BlueScale 服务正在使用这个数据目录。请先停止服务后重试。")
		return 1
	}
	if errors.Is(err, app.ErrAdministratorNotConfigured) {
		fmt.Fprintln(stderr, "恢复操作失败：该数据库尚未配置管理员。")
		return 1
	}
	fmt.Fprintf(stderr, "恢复操作失败：%v\n", err)
	return 1
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, "用法：")
	fmt.Fprintln(output, "  bluescale                         启动服务")
	fmt.Fprintln(output, "  bluescale admin <command> [选项]  管理员本地恢复")
	fmt.Fprintln(output, "  bluescale help                    显示帮助")
}

func printAdminUsage(output io.Writer) {
	fmt.Fprintln(output, "管理员恢复命令：")
	fmt.Fprintln(output, "  bluescale admin show-username [--data-dir DIR]")
	fmt.Fprintln(output, "  bluescale admin reset-username [--data-dir DIR] [--yes] NEW_USERNAME")
	fmt.Fprintln(output, "  bluescale admin reset-password [--data-dir DIR] [--yes] [--password-stdin] [--revoke-api-tokens]")
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func displayURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

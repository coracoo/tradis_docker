package admincli

import (
	"bufio"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/settings"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/term"
)

func Run(args []string, in io.Reader, out io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "admin" {
		return false, nil
	}
	if len(args) != 2 || args[1] != "reset-password" {
		return true, fmt.Errorf("用法: backend admin reset-password")
	}

	password, confirmation, err := readPasswordPair(in, out)
	if err != nil {
		return true, err
	}
	if password != confirmation {
		return true, fmt.Errorf("两次输入的密码不一致")
	}
	if err := database.ResetAdminPasswordAt(
		filepath.Join(settings.GetDataDir(), "data.db"),
		password,
	); err != nil {
		return true, err
	}
	_, _ = fmt.Fprintln(out, "管理员密码已重置。")
	return true, nil
}

func readPasswordPair(in io.Reader, out io.Writer) (string, string, error) {
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		first, err := readTerminalPassword(file, out, "新密码: ")
		if err != nil {
			return "", "", err
		}
		second, err := readTerminalPassword(file, out, "再次输入新密码: ")
		if err != nil {
			return "", "", err
		}
		return first, second, nil
	}

	scanner := bufio.NewScanner(in)
	_, _ = fmt.Fprint(out, "新密码: ")
	if !scanner.Scan() {
		return "", "", fmt.Errorf("读取新密码失败")
	}
	first := scanner.Text()
	_, _ = fmt.Fprint(out, "再次输入新密码: ")
	if !scanner.Scan() {
		return "", "", fmt.Errorf("读取确认密码失败")
	}
	return first, scanner.Text(), nil
}

func readTerminalPassword(file *os.File, out io.Writer, prompt string) (string, error) {
	_, _ = fmt.Fprint(out, prompt)
	value, err := term.ReadPassword(int(file.Fd()))
	_, _ = fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("读取密码失败: %w", err)
	}
	return string(value), nil
}

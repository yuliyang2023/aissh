package desktop

import (
	"runtime"
	"strconv"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ApplicationMenu uses the platform's actual menu bar, including its standard
// editing/window roles. Actions only select UI workflows; they never save a
// credential or approve a connection without the corresponding UI interaction.
func ApplicationMenu(s *Service) *menu.Menu {
	appMenu := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		appMenu.Append(menu.AppMenu())
	}
	action := func(command string) menu.Callback {
		return func(*menu.CallbackData) { s.emit("desktop-command", command) }
	}
	file := appMenu.AddSubmenu("文件")
	file.AddText("新建 AI 对话…", keys.CmdOrCtrl("n"), action("new-conversation"))
	file.AddText("新建主机…", nil, action("new-host"))
	file.AddText("新建凭据…", keys.Combo("n", keys.CmdOrCtrlKey, keys.ShiftKey), action("new-credential"))
	file.AddText("新建系统连接…", nil, action("new-resource"))
	file.AddText("新建 AI 配置…", nil, action("new-ai"))
	file.AddSeparator()
	file.AddText("导入 SSH 配置…", keys.Combo("i", keys.CmdOrCtrlKey, keys.ShiftKey), action("import"))
	if runtime.GOOS != "darwin" {
		file.AddSeparator()
		file.AddText("退出", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { wails.Quit(s.ctx) })
	}
	appMenu.Append(menu.EditMenu())
	view := appMenu.AddSubmenu("显示")
	for i, page := range []struct{ name, command string }{
		{"AI 会话", "conversations"}, {"SSH 会话", "sessions"}, {"主机", "hosts"}, {"凭据", "credentials"}, {"系统连接", "resources"}, {"AI 配置", "ai"},
	} {
		if i == 2 {
			view.AddSeparator()
		}
		view.AddText(page.name, keys.CmdOrCtrl(strconv.Itoa(i+1)), action("page:"+page.command))
	}
	view.AddSeparator()
	view.AddText("显示 / 隐藏侧栏", keys.Combo("s", keys.CmdOrCtrlKey, keys.OptionOrAltKey), action("sidebar"))
	view.AddText("搜索", keys.CmdOrCtrl("f"), action("search"))
	view.AddText("刷新配置", keys.CmdOrCtrl("r"), action("refresh"))
	appMenu.Append(menu.WindowMenu())
	return appMenu
}

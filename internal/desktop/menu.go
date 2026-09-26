package desktop

import (
	"runtime"

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
	file.AddText("新建会话…", keys.CmdOrCtrl("n"), action("new-conversation"))
	file.AddText("新建主机…", nil, action("new-host"))
	file.AddText("新建凭据…", keys.Combo("n", keys.CmdOrCtrlKey, keys.ShiftKey), action("new-credential"))
	file.AddText("新建外联系统…", nil, action("new-resource"))
	file.AddText("新建本地知识…", nil, action("new-knowledge"))
	file.AddText("新建 AI 配置…", nil, action("new-ai"))
	file.AddSeparator()
	file.AddText("导入 SSH 配置…", keys.Combo("i", keys.CmdOrCtrlKey, keys.ShiftKey), action("import"))
	if runtime.GOOS != "darwin" {
		file.AddSeparator()
		file.AddText("退出", keys.CmdOrCtrl("q"), func(*menu.CallbackData) { wails.Quit(s.ctx) })
	}
	appMenu.Append(menu.EditMenu())
	view := appMenu.AddSubmenu("显示")
	view.AddText("工作区", keys.CmdOrCtrl("1"), action("workspace"))
	view.AddText("外联系统", nil, action("page:resources"))
	view.AddText("本地知识", nil, action("page:knowledge"))
	view.AddText("告警记录与桌宠…", nil, action("monitoring"))
	view.AddText("告警任务…", nil, action("monitor-inventory"))
	view.AddText("设置…", keys.CmdOrCtrl(","), action("settings"))
	view.AddSeparator()
	view.AddText("展开 / 收起会话列表", keys.Combo("s", keys.CmdOrCtrlKey, keys.OptionOrAltKey), action("sidebar"))
	view.AddText("搜索", keys.CmdOrCtrl("f"), action("search"))
	view.AddText("刷新配置", keys.CmdOrCtrl("r"), action("refresh"))
	appMenu.Append(menu.WindowMenu())
	return appMenu
}

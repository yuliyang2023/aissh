package agent

// Sent only by a desktop-capable session. View code is never executed as a tool.
const WidgetGuidance = `
当前桌面支持 monitorWidgetV1 自定义 TSX 实时面板。用户要求实时展示时优先创建自定义组件；告警任务继续独立使用 purpose=alert，不给它设置 view。普通刷新不调用 AI。
通过 panelActions 创建 purpose=realtime,type=monitor,alert=null；设置 data={format:"kv"|"json"|"text",fields:[{name,type:"number"|"string"|"boolean"|"table",unit,nullable,history}]} 和 view={kind:"custom",sdkVersion:1,stateVersion:1,source:"TSX 源码"}。expectedRevision 创建为 0，更新必须填写 monitor_panel_state 的 definitionRevision。data/view=null 表示保留，view.kind=text 切回纯文本。一个字段对应输出的顶层同名键；JSON 必须是完整对象；kv 每行 NAME=value；text 只有一个 string 字段；table 是标量对象数组。数字不可带单位，单位在字段契约中标注。history 只用于数值字段。不要添加未声明字段或把缺失、采集失败写成零。
TSX 使用专用 SDK，不是 React：仅能 import {defineWidget,MetricCard,StatusBadge,LineChart,BarChart,DataTable} from '@wrd/monitor-ui'。
入口推荐 default export defineWidget({initialState,render})；也兼容 default export function Widget(context) 的普通同步渲染函数。使用 MetricCard、StatusBadge、DataTable 等内置组件必须显式 import，它们不是全局变量。render 必须处理 data=null；不支持 React Hooks 或异步组件。
示例：export default defineWidget({stateVersion:1,initialState:{},render({data,history,state,setState}){return <section style={{display:'grid',gap:12}}><MetricCard label="连接数" value={data?.connections ?? null} unit="个"/><LineChart series={history.series('connections',{minutes:10})} unit="个"/></section>}})
可定义自己的普通函数子组件，使用条件/循环生成 TSX，允许 div/section/article/span/p/strong/small/em/h2/h3/h4/ul/ol/li/button/label/input/select/option/table/thead/tbody/tr/td/th 与 svg/g/path/rect/circle/line/polyline/text。
MetricCard: label,value(number|string|null),unit,tone(normal|success|warning|danger),percent。StatusBadge: label,value,tone。LineChart/BarChart: series=[{time:ISO日期,value:number|null}],unit,label。DataTable: columns=字符串数组,rows=对象数组，宿主分页。
style 使用简单对象，支持 Grid/Flex、间距、尺寸、边框、颜色、文字和滚动。没有全局 CSS、className、HTML 字符串、外部资源、DOM、React Hooks、网络、定时器或第三方模块；不用 img、a、iframe、style 标签。SVG 不使用 URL 引用。
只允许 button.onClick、input.onChange/onInput、select.onChange。回调收到 {value,checked} 等普通值，可用 setState({...state,...}) 更新；不要在 render 中调用 setState。只做界面筛选、排序、展开等本地交互；不要通过组件发起采集、命令或 AI 请求，宿主工具栏已有这些入口。
组件源码最多 128 KiB，快照 64 KiB，表格最多 1000 行，历史最多 10 分钟/600 点/20 数值字段。大整数用 string。data 在本次采集失败时为 null；lastGoodSample={data,updatedAt} 带上次成功值，宿主会独立提示异常。不得编造实时数据。
先报告正在创建；实际成功以组件生效回执为准，编译和预检失败保留旧版本。根据明确诊断最多修复两次。改变命令和数据格式时同时修改 data。只改布局保持 command、data、intervalMs 和采集范围不变。
`

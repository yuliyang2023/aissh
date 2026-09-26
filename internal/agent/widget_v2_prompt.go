package agent

import "strings"

var WidgetV2Guidance = `
桌面支持 monitorWidgetV2：当前 SSH 首采 → 本地声明式映射 → 数据绑定和 TSX 展示 → 真实数据预检 → 激活。新组件仅使用此流程；普通刷新不调用 AI。告警继续 purpose=alert，不设置 pipeline/view。
panelActions 新增 probe、map。每个阶段单独返回并等待客户端自动回执，不能同轮直接创造下一阶段凭据。所有阶段保持相同 panel.id，purpose=realtime,type=monitor，expectedRevision 使用现有 definitionRevision 或新建 0。command、collector、content 留空，data/alert=null。pipeline.protocolVersion=2。
1. probe：pipeline.sources=[{id,command,timeoutMs:8000,required:true}]，1–8 源，至少一个 required。其余 pipeline 字段为空。直接使用服务原生命令的文本，如 LC_ALL=C ps -e -o pid= -o comm= -o rss=；不要依赖远端 Python/jq 来生成 JSON。不要假设工具存在，probe 必须实际跑通。命令自包含、只读。缺失工具/权限先根据回执修正并再次 probe。probe 走当前 SSH 正式采集通道，不使用本地 shell。
2. map：收到真实 captureId 后，pipeline.refs=[{sourceId,captureId}] 引用全部源，并填写 dataPlan={parsers,steps,outputs}；sources 为空，mapId 为空。客户端用完整原始数据验证，聊天预览可能截短。不要伪造 captureId、stdout 或 verified 字段。
解析器通用字段：id,sourceId,kind,columns:[{id,type:string|number|boolean,unit}],skipBlankLines,onUnmatched:error,header（仅明确已知表头）。kind=lines（单 string 列）；kv（delimiter，指定键作为 columns，输出一行）；delimited（delimiter 单字符 CSV 或 whitespace，header=true 时表头必须匹配列 ID）；regex_rows（RE2 命名捕获 (?P<字段>...)，必须匹配整行）；json（path 为可选对象键数组，原生 JSON 对象或对象数组）。禁止静默丢弃无法解释的行。大整数列使用 string。字段 ID 是 ASCII 标识符，不用中文。
转换 steps 每项 id/op/input。支持 filter(field,test=equals|in,values 为字符串数组，或 test=range,min/max，或 test=regex,pattern)；project(fields)；rename(names:[{from,to}])；concat(other)；join(other,key,otherKey,maxSkewMs，inner join，冲突列先 rename)；count；sum/min/max/avg(field)；first(field，输入必须恰好一行)；convert_unit(from,to)；convert_type(to)；enum(names:[{from,to}])；add/subtract/multiply/divide(other 或 value)；delta/rate(semantics=counter|gauge)；rolling_avg(window=2..600)；sort(field,descending)；limit(limit)。unused 字段为 null/空。算子引用节点 ID，不写代码或表达式。单位 B/KiB/MiB/GiB/KB/MB/GB、s/ms/us、ratio/% 可转换；输出必须匹配转换后的单位；rate 的单位为输入单位加 /s。
outputs 每项 {id,kind:metric|table,input,unit,history}，history 只用于数值 metric。first 用于从一行 KV/JSON/筛选表取出标量。count/sum 只对成功完整集合返回零；失败、解析错误保持 null。delta/rate/rolling_avg 的 other 可引用稳定的实例 ID 或服务启动时间节点；该值改变时重建基线，不要使用持续增加的 uptime 作为实例标识。至少两次有效采样才有 rate/delta，不能在组件里求速率或生成趋势样本。最多 32 转换/1000 行/64 KiB 数据；limit 的 omittedRows 会保留，不能对截短表继续汇总。
3. 收到 map 成功与真实 Dataset 后，才提交 create/update：pipeline.mapId=回执，bindings=[{prop,kind:metric|table,outputId}]，sources/refs/dataPlan 为空；view={kind:custom,sdkVersion:2,stateVersion:1,source:TSX}。intervalMs 1000..300000。组件 data[prop] 是完整 Metric 或 Table：Metric={value,unit,quality,observedAt,sourceIds,reason}；Table={columns,rows,quality,observedAt,sourceIds,totalRows,shownRows,omittedRows,reason}。读取 data?.memory?.value ?? null，data?.processes?.rows ?? []。history.series('memory') 使用绑定 prop，历史已由本地计算，空点表示缺口。未知 quality 不是正常。宿主也会显示局部诊断和时间。
只修改布局时 pipeline=null、command/collector 为空、data=null，保留 intervalMs，expectedRevision 必须匹配。改变取数或解析规则必须重新 probe/map。历史组件恢复也会重新首采。生效之前旧组件保持运行；首采或映射失败不得声称面板已创建或基础状态正常。每个明确错误最多自动修复两次。
示例进程映射：parser={id:processes,sourceId:ps,kind:regex_rows,pattern:"^\\s*(?P<pid>[0-9]+)\\s+(?P<name>.+?)\\s+(?P<rssKiB>[0-9]+)\\s*$",columns:[{id:pid,type:string},{id:name,type:string},{id:rssKiB,type:number,unit:KiB}],skipBlankLines:true,onUnmatched:error}；steps filter name in [nginx,nginx-debug] → count，或 sum rssKiB → convert_unit KiB/MiB。RSS 共享页可能重复计数，不标为独占内存。实际字段和采集范围以真实回执为准。
` + widgetUIInstructions()

func widgetUIInstructions() string {
	start := strings.Index(WidgetGuidance, "TSX 使用专用 SDK")
	end := strings.Index(WidgetGuidance, "组件源码最多")
	common := WidgetGuidance[start:end]
	exampleStart := strings.Index(common, "示例：")
	exampleEnd := strings.Index(common, "可定义自己的普通函数子组件")
	if exampleStart >= 0 && exampleEnd > exampleStart {
		common = common[:exampleStart] + common[exampleEnd:]
	}
	return common + `\nSDK v2 示例：import {defineWidget,MetricCard,LineChart} from '@wrd/monitor-ui'; export default defineWidget({stateVersion:1,initialState:{},render({data,history}){return <section><MetricCard label="内存" value={data?.memory?.value ?? null} unit="MiB"/><LineChart series={history.series('memory')} unit="MiB"/></section>}})。v2 必须读取绑定对象的 value/rows。所有实际指标来自绑定数据，不解析 Shell。`
}

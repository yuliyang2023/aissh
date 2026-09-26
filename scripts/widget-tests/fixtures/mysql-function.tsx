export default function MySQLStatus(props: any) {
  const data = props && props.data;
  if (!data) {
    return (
      <section style={{ padding: 12 }}>
        <strong>暂无有效采集数据</strong>
      </section>
    );
  }
  const services = Array.isArray(data.services) ? data.services : [];
  return (
    <section
      style={{
        padding: 12,
        display: "flex",
        flexDirection: "column",
        gap: 10,
        minWidth: 0,
      }}
    >
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 8,
          alignItems: "center",
        }}
      >
        <strong
          style={{ color: data.response === "alive" ? "#15803d" : "#b45309" }}
        >
          本地响应：{data.response === "alive" ? "有响应" : "未知"}
        </strong>
        <small>默认套接字 · 每 15 秒采样</small>
      </div>
      {data.diagnostic ? (
        <p
          style={{
            margin: 0,
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            color: "#b45309",
          }}
        >
          {data.diagnostic}
        </p>
      ) : null}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
          gap: 10,
        }}
      >
        <div>
          <small>运行时长（秒）</small>
          <div>
            <strong>
              {data.uptime_seconds == null ? "未知" : data.uptime_seconds}
            </strong>
          </div>
        </div>
        <div>
          <small>连接线程（个）</small>
          <div>
            <strong>{data.threads == null ? "未知" : data.threads}</strong>
          </div>
        </div>
        <div>
          <small>慢查询累计（次）</small>
          <div>
            <strong>
              {data.slow_queries == null ? "未知" : data.slow_queries}
            </strong>
          </div>
        </div>
        <div>
          <small>启动以来平均 QPS（次/秒）</small>
          <div>
            <strong>{data.avg_qps == null ? "未知" : data.avg_qps}</strong>
          </div>
        </div>
      </div>
      {services.length > 0 ? (
        <div style={{ overflow: "auto" }}>
          <table
            style={{
              width: "100%",
              borderCollapse: "collapse",
              textAlign: "left",
            }}
          >
            <thead>
              <tr>
                <th>服务</th>
                <th>状态</th>
                <th>PID</th>
                <th>自动重启（次）</th>
              </tr>
            </thead>
            <tbody>
              {services.map((service: any) => (
                <tr key={service.unit}>
                  <td style={{ padding: 4 }}>{service.unit}</td>
                  <td style={{ padding: 4 }}>
                    {service.state}/{service.substate}
                  </td>
                  <td style={{ padding: 4 }}>
                    {service.pid == null ? "未知" : service.pid}
                  </td>
                  <td style={{ padding: 4 }}>
                    {service.restarts == null ? "未知" : service.restarts}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <small>systemd 服务状态不可用</small>
      )}
    </section>
  );
}

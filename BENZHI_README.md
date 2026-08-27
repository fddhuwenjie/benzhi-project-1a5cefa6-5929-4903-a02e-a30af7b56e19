# BENZHI_README

## 项目说明
- 项目：benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19
- 项目用途：完整实现排烟联动演练资格工作台，覆盖冻结方案、事件采集、确定性判定、偏差整改、定向复演、独立复核、证书签发及审计校验。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：排烟联动演练资格工作台
- 项目介绍：面向建筑安全团队的排烟系统联动演练资格裁定应用，以可追溯的事件时序、确定性规则、偏差整改和独立复核形成不可变启用证书。
- 项目概述：面向建筑安全团队的排烟系统联动演练资格裁定应用，以可追溯的事件时序、确定性规则、偏差整改和独立复核形成不可变启用证书。
- 核心工作流：单个演练案件从草稿建档开始，冻结分区、设备和时限规则后进入采集；记录员提交一次演练中探测器、风机、风阀、防火门和压差确认的有序事件，系统执行确定性判定。失败项必须形成偏差、登记纠正措施并完成定向复演，全部规则合格后转交未参与记录的复核员批准或拒绝，终态封存可校验的资格证书。状态依次受控为 draft、protocol_frozen、collecting、evaluation_failed、correcting、reperforming、review_pending、qualified 或 rejected。
- 对外接口：Go 服务直接提供原生 HTML、CSS 和 JavaScript 的同源浏览器工作台；页面以案件状态条、设备分区表、毫秒级事件时间轴、规则结果、偏差整改区、复核区和证书校验视图完成唯一主流程，不引入 Node 构建链，也不把同源 JSON 端点作为独立产品界面。服务支持 -addr=127.0.0.1:<port>，默认监听 127.0.0.1:19081，禁止默认绑定 8080、80、3000 或 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19-arm64 linux/arm64

docker run -it benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19081`

const $ = (query, root = document) => root.querySelector(query);
const $$ = (query, root = document) => [...root.querySelectorAll(query)];
const statuses = ["draft", "protocol_frozen", "collecting", "evaluation_failed", "correcting", "reperforming", "review_pending", "qualified", "rejected"];
const labels = {draft:"草稿", protocol_frozen:"方案冻结", collecting:"采集中", evaluation_failed:"评估失败", correcting:"整改中", reperforming:"定向复演", review_pending:"待复核", qualified:"已合格", rejected:"已拒绝"};
const defaultDevices = [["DET-01","东区","detector"],["FAN-01","核心区","fan"],["DAMP-01","东区","damper"],["DOOR-01","核心区","door"],["PRESS-01","东区","pressure"]];
const defaultRules = [["R-FAN","探测至风机启动","detector","fan","3000","photo"],["R-DAMP","探测至风阀到位","detector","damper","4000","photo"],["R-DOOR","探测至防火门闭合","detector","door","5000","photo"],["R-PRESS","风机至压差确认","fan","pressure","6000","meter"]];
const eventTypesByRole = {detector:["detected"], fan:["started"], damper:["opened","positioned"], door:["closed"], pressure:["confirmed"]};
let current = null;
let precheckDigest = "";
let eventCatalogSignature = "";

class APIError extends Error {
  constructor(response, body) {
    super(body.error?.message || `请求失败 (${response.status})`);
    this.status = response.status;
    this.detail = body.error || {};
  }
}
const requestID = () => crypto.randomUUID();
const split = value => String(value || "").split(",").map(item => item.trim()).filter(Boolean);
const escapeHTML = value => String(value ?? "").replace(/[&<>"']/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"})[char]);
function note(message, error = false) { const node = $("#notice"); node.textContent = message; node.className = error ? "error" : ""; }

async function api(path, options = {}) {
  const response = await fetch(path, {headers:{"Content-Type":"application/json"}, ...options});
  let body = {};
  try { body = await response.json(); } catch {}
  if (!response.ok) throw new APIError(response, body);
  return body;
}
function meta() { return {request_id:requestID(), expected_revision:current?.Revision || 0}; }
async function command(path, payload) {
  const caseID = current?.CaseID || payload.case_id;
  const pendingKey = `pending:${caseID}:${payload.request_id}`;
  sessionStorage.setItem(pendingKey, JSON.stringify({path, payload}));
  try {
    const result = await api(path, {method:"POST", body:JSON.stringify(payload)});
    sessionStorage.removeItem(pendingKey);
    return result;
  } catch (error) {
    if (error instanceof TypeError && caseID) {
      const receipt = await api(`/api/cases/${encodeURIComponent(caseID)}/requests/${encodeURIComponent(payload.request_id)}`);
      if (receipt.processed && receipt.result) {
        sessionStorage.removeItem(pendingKey);
        note(`已通过回执恢复提交结果 · ${receipt.response_summary}`);
        return receipt.result;
      }
    }
    if (error instanceof APIError && error.status === 409 && error.detail.current_revision && caseID) {
      await load(caseID, false);
      error.message = `${error.message}；案件已刷新至修订 ${error.detail.current_revision}，请核对后重新提交`;
    }
    throw error;
  }
}
function activate(id) {
  $$(".tabs button").forEach(button => button.classList.toggle("active", button.dataset.tab === id));
  $$(".panel").forEach(panel => panel.classList.toggle("active", panel.id === id));
}
$$(".tabs button").forEach(button => button.addEventListener("click", () => activate(button.dataset.tab)));

$("#device-editor").innerHTML = defaultDevices.map(values => `<tr>${values.map((value, i) => `<td><input data-k="${["ID","Zone","Role"][i]}" value="${escapeHTML(value)}"></td>`).join("")}</tr>`).join("");
$("#rule-editor").innerHTML = defaultRules.map(values => `<tr>${values.map((value, i) => `<td><input data-k="${["ID","Name","FromRole","ToRole","MaxResponseMS","RequiredEvidence"][i]}" value="${escapeHTML(value)}" ${i === 4 ? 'type="number"' : ""}></td>`).join("")}</tr>`).join("");

function deviceCatalog() {
  return current?.Protocol?.Devices || readRows("#device-editor tr");
}
function addEvent(values = {}) {
  const row = document.createElement("div");
  row.className = "event-row";
  const catalog = deviceCatalog();
  const selectedDevice = values.DeviceID || catalog[0]?.ID || "";
  const options = catalog.map(device => `<option value="${escapeHTML(device.ID)}">${escapeHTML(device.ID)} · ${escapeHTML(device.Role)}</option>`).join("");
  const now = values.At || new Date().toISOString();
  row.innerHTML = `<label>设备<select data-k="DeviceID" required>${options}</select></label><label>事件类型<select data-k="EventType"></select></label><label>时间<input data-k="At" type="datetime-local" step="0.001" value="${now.slice(0,23)}" required></label><label>证据种类<select data-k="Kind"><option>photo</option><option>meter</option></select></label><label>证据引用<input data-k="URI" value="evidence://现场记录" required></label><label>SHA-256<input data-k="SHA256" value="${values.SHA256 || "a".repeat(64)}" required></label><button type="button" title="删除事件">×</button>`;
  const deviceSelect = $('[data-k="DeviceID"]', row);
  deviceSelect.value = selectedDevice;
  if (!deviceSelect.value && catalog.length) deviceSelect.value = catalog[0].ID;
  const updateTypes = () => {
    const role = catalog.find(device => device.ID === deviceSelect.value)?.Role;
    const types = eventTypesByRole[role] || ["detected","started","opened","positioned","closed","confirmed"];
    const selected = values.EventType || types[0];
    const typeSelect = $('[data-k="EventType"]', row);
    typeSelect.innerHTML = types.map(type => `<option value="${type}">${type}</option>`).join("");
    typeSelect.value = types.includes(selected) ? selected : types[0];
  };
  deviceSelect.onchange = updateTypes;
  updateTypes();
  $('[data-k="Kind"]', row).value = values.Kind || "photo";
  $("button", row).onclick = () => row.remove();
  $("#event-editor").append(row);
}
$("#add-event").onclick = () => addEvent();
addEvent();
addEvent({DeviceID:"FAN-01", EventType:"started", At:new Date(Date.now()+1500).toISOString()});

function readRows(selector) {
  return $$(selector).map(row => Object.fromEntries($$("input", row).map(input => [input.dataset.k, input.dataset.k === "MaxResponseMS" ? Number(input.value) : input.value])));
}
function protocolPayload() {
  const form = $("#freeze-form");
  const data = new FormData(form);
  return {frozen_by:data.get("frozen_by"), zones:split(data.get("zones")), participant_ids:split(data.get("participants")), required_evidence_kinds:["photo","meter"], devices:readRows("#device-editor tr"), rules:readRows("#rule-editor tr")};
}
function clearFieldErrors() { $$(".field-error").forEach(node => node.classList.remove("field-error")); }
function locateIssues(issues) {
  clearFieldErrors();
  for (const issue of issues || []) {
    let node = null;
    if (issue.field.startsWith("devices.") && issue.row) node = $$("#device-editor tr")[issue.row-1]?.querySelector(`[data-k="${issue.field.endsWith("id") ? "ID" : issue.field.endsWith("zone") ? "Zone" : "Role"}"]`);
    if (issue.field.startsWith("rules.") && issue.row) {
      const key = {id:"ID", from_role:"FromRole", to_role:"ToRole", max_response_ms:"MaxResponseMS", required_evidence:"RequiredEvidence"}[issue.field.split(".").pop()];
      node = $$("#rule-editor tr")[issue.row-1]?.querySelector(`[data-k="${key}"]`);
    }
    node?.classList.add("field-error");
  }
}
function renderPrecheck(check) {
  locateIssues(check.issues);
  if (!check.valid) {
    $("#precheck-view").innerHTML = `<strong class="fail">预检发现 ${check.issues.length} 项问题</strong><ol>${check.issues.map(issue => `<li>${issue.row ? `第 ${issue.row} 行 · ` : ""}${escapeHTML(issue.message)}</li>`).join("")}</ol>`;
    return;
  }
  const summary = check.summary;
  const roles = Object.entries(summary.device_role_distribution).sort().map(([role,count]) => `${role} ${count}`).join(" · ");
  $("#precheck-view").innerHTML = `<strong class="pass">预检通过</strong><p>${summary.zone_count} 个分区 · ${roles} · ${summary.rule_count} 条规则 · 证据 ${summary.required_evidence_kinds.join(", ")}</p><small>${summary.digest}</small>`;
}
$("#freeze-form").addEventListener("input", () => { precheckDigest = ""; $("#freeze-btn").disabled = true; });
$("#precheck-btn").onclick = async () => {
  if (!current) return note("请先建立或打开草稿案件", true);
  try {
    const output = await api(`/api/cases/${encodeURIComponent(current.CaseID)}/protocol/precheck`, {method:"POST", body:JSON.stringify({...meta(), ...protocolPayload()})});
    renderPrecheck(output.check);
    precheckDigest = output.check.valid ? output.check.summary.digest : "";
    $("#freeze-btn").disabled = !precheckDigest;
    note(output.check.valid ? "方案预检通过，可确认冻结" : "请按定位问题修正方案", !output.check.valid);
  } catch (error) { note(error.message, true); }
};

async function load(id, announce = true) {
  current = await api(`/api/cases/${encodeURIComponent(id)}`);
  render();
  if (announce) note("案件数据已刷新");
}
$("#open-form").onsubmit = async event => { event.preventDefault(); try { await load($("#open-id").value.trim()); } catch (error) { note(error.message, true); } };
$("#create-form").onsubmit = async event => {
  event.preventDefault();
  const data = new FormData(event.target);
  const payload = {...meta(), case_id:data.get("case_id"), building_name:data.get("building_name"), created_by:data.get("created_by")};
  try { await command("/api/cases", payload); await load(payload.case_id, false); $("#open-id").value = payload.case_id; note("案件已建立，请先预检方案"); } catch (error) { note(error.message, true); }
};
$("#freeze-form").onsubmit = async event => {
  event.preventDefault();
  if (!precheckDigest) return note("冻结前必须完成当前内容的方案预检", true);
  const payload = {...meta(), ...protocolPayload(), precheck_digest:precheckDigest};
  try { await command(`/api/cases/${encodeURIComponent(current.CaseID)}/freeze`, payload); await load(current.CaseID, false); activate("timeline"); note("方案已按预检摘要冻结"); } catch (error) { if (error.detail?.issues) locateIssues(error.detail.issues); note(error.message, true); }
};

function readEvents() {
  return $$(".event-row").map(row => {
    const values = Object.fromEntries($$("[data-k]", row).map(input => [input.dataset.k, input.value]));
    return {DeviceID:values.DeviceID, EventType:values.EventType, At:new Date(values.At).toISOString(), EvidenceRefs:[{Kind:values.Kind, URI:values.URI, SHA256:values.SHA256}]};
  });
}
function clientValidateEvents(events) {
  $$(".event-row").forEach(row => row.classList.remove("row-error"));
  const duplicate = new Map();
  const uriDigest = new Map();
  const digestURI = new Map();
  const problems = [];
  events.forEach((event, index) => {
    const instant = new Date(event.At).getTime();
    const key = `${event.DeviceID}|${event.EventType}|${instant}`;
    if (duplicate.has(key)) problems.push([index, `与第 ${duplicate.get(key)+1} 项事件重复`]); else duplicate.set(key,index);
    for (const ref of event.EvidenceRefs) {
      const digest = ref.SHA256.toLowerCase();
      if (uriDigest.has(ref.URI) && uriDigest.get(ref.URI) !== digest) problems.push([index,"相同 URI 对应不同摘要"]); else uriDigest.set(ref.URI,digest);
      if (digestURI.has(digest) && digestURI.get(digest) !== ref.URI) problems.push([index,"相同摘要对应不同 URI"]); else digestURI.set(digest,ref.URI);
    }
  });
  problems.forEach(([index]) => $$(".event-row")[index]?.classList.add("row-error"));
  return problems;
}
$("#run-form").onsubmit = async event => {
  event.preventDefault();
  const data = new FormData(event.target);
  const events = readEvents();
  const problems = clientValidateEvents(events);
  if (problems.length) return note(`时间轴存在冲突：${problems.map(item => item[1]).join("；")}`, true);
  const payload = {...meta(), run_id:data.get("run_id"), run_kind:data.get("run_kind"), recorded_by:data.get("recorded_by"), target_rule_ids:split(data.get("target_rules")), events};
  try { await command(`/api/cases/${encodeURIComponent(current.CaseID)}/runs`, payload); await load(current.CaseID, false); activate("rules"); note("轮次已原子写入并完成评估"); } catch (error) { if (error.detail?.issues) error.detail.issues.forEach(issue => issue.row && $$(".event-row")[issue.row-1]?.classList.add("row-error")); note(error.message, true); }
};

$("#correction-form").onsubmit = async event => {
  event.preventDefault();
  const deviations = $$(".deviation-card[data-rule]").map(card => ({deviation_id:card.dataset.deviation || `${card.dataset.rule}-${Date.now()}`, rule_id:card.dataset.rule, root_cause:$("[data-k='cause']",card).value, corrective_action:$("[data-k='action']",card).value, scope_rule_ids:$$('[data-k="scope"]:checked',card).map(input => input.value)}));
  try {
    await command(`/api/cases/${encodeURIComponent(current.CaseID)}/deviations`, {...meta(), deviations});
    await load(current.CaseID, false);
    activate("timeline");
    $("#run-form [name=run_kind]").value = "targeted";
    $("#run-form [name=target_rules]").value = allowedTargets().join(",");
    note("整改措施已追加，等待授权范围内的定向复演");
  } catch (error) { note(error.message, true); }
};
$("#failure-filter").onchange = renderRules;
$("#critical-filter").onchange = renderRules;

let readinessTimer = 0;
async function loadReadiness() {
  if (!current || current.Status !== "review_pending") return;
  const reviewer = $("#review-form [name=reviewer_id]").value.trim();
  try { renderReadiness(await api(`/api/cases/${encodeURIComponent(current.CaseID)}/review-readiness?reviewer_id=${encodeURIComponent(reviewer)}`)); } catch (error) { note(error.message,true); }
}
$("#review-form [name=reviewer_id]").oninput = () => { clearTimeout(readinessTimer); readinessTimer = setTimeout(loadReadiness, 180); };
$("#review-form").onsubmit = async event => {
  event.preventDefault();
  const data = new FormData(event.target);
  try { await command(`/api/cases/${encodeURIComponent(current.CaseID)}/review`, {...meta(), reviewer_id:data.get("reviewer_id"), decision:data.get("decision"), reason:data.get("reason")}); await load(current.CaseID, false); note("复核结论与清单摘要已封存"); } catch (error) { note(error.message,true); }
};
$("#verify-btn").onclick = async () => {
  try {
    const response = await fetch(`/api/cases/${encodeURIComponent(current.CaseID)}/verify`);
    const verification = await response.json();
    renderVerification(verification);
  } catch (error) { note(error.message,true); }
};

function render() {
  if (!current) return;
  $("#case-title").textContent = `${current.CaseID} · ${current.BuildingName}`;
  $("#revision").textContent = current.Revision;
  $("#protocol-badge").textContent = current.Protocol ? "已冻结" : "未冻结";
  $("#create-form").classList.add("hidden");
  $("#freeze-form").classList.toggle("hidden", !!current.Protocol);
  const index = statuses.indexOf(current.Status);
  $("#status-strip").innerHTML = statuses.map((status,i) => `<li class="${status === current.Status ? "current" : i < index ? "done" : ""}">${labels[status]}</li>`).join("");
  renderProtocol(); renderTimeline(); renderRules(); renderDeviations(); renderCertificate();
}
function renderProtocol() {
  if (!current.Protocol) { $("#protocol-view").innerHTML = ""; return; }
  const protocol = current.Protocol;
  $("#protocol-view").innerHTML = `<h3>冻结基线 ${escapeHTML(protocol.ProtocolID)}</h3><p><small>${escapeHTML(protocol.BaselineDigest)}</small></p><div class="table-wrap"><table><thead><tr><th>设备</th><th>分区</th><th>角色</th></tr></thead><tbody>${protocol.Devices.map(device => `<tr><td>${escapeHTML(device.ID)}</td><td>${escapeHTML(device.Zone)}</td><td>${escapeHTML(device.Role)}</td></tr>`).join("")}</tbody></table></div>`;
}
function renderTimeline() {
  const runs = current.Runs || [];
  syncEventCatalog();
  $("#run-form").classList.toggle("hidden", !["collecting","reperforming"].includes(current.Status));
  $("#run-form [name=run_id]").value = `RUN-${runs.length+1}`;
  if (current.Status === "reperforming") { $("#run-form [name=run_kind]").value = "targeted"; $("#run-form [name=target_rules]").value = allowedTargets().join(","); }
  $("#timeline-view").innerHTML = runs.map(run => `<h3>${escapeHTML(run.RunID)} · ${escapeHTML(run.RunKind)}</h3><div class="table-wrap"><table><thead><tr><th>时间</th><th>设备</th><th>事件</th><th>证据摘要</th></tr></thead><tbody>${run.Events.map(event => `<tr><td>${new Date(event.At).toLocaleString("zh-CN",{fractionalSecondDigits:3})}</td><td>${escapeHTML(event.DeviceID)}</td><td>${escapeHTML(event.EventType)}</td><td>${escapeHTML(event.EvidenceRefs?.[0]?.SHA256 || "—")}</td></tr>`).join("")}</tbody></table></div>`).join("");
}
function syncEventCatalog() {
  if (!current.Protocol) return;
  const signature = JSON.stringify(current.Protocol.Devices.map(device => [device.ID, device.Role]));
  if (signature === eventCatalogSignature) return;
  const previous = $$(".event-row").map(row => Object.fromEntries($$("[data-k]", row).map(input => [input.dataset.k, input.value])));
  $("#event-editor").innerHTML = "";
  previous.forEach(values => addEvent(values));
  eventCatalogSignature = signature;
}
function renderRules() {
  let results = current?.Evaluations || [];
  const passed = results.filter(result => result.Passed).length;
  $("#result-summary").textContent = results.length ? `${passed}/${results.length} 合格` : "尚未评估";
  const category = $("#failure-filter").value;
  if (category) results = results.filter(result => result.FailureCategory === category);
  if ($("#critical-filter").checked) results = results.filter(result => result.CriticalPass);
  $("#rules-view").innerHTML = results.length ? `<div class="table-wrap"><table><thead><tr><th>规则</th><th>结论</th><th>诊断</th><th>裕量</th><th>候选事件</th></tr></thead><tbody>${results.map(result => `<tr><td><strong>${escapeHTML(result.RuleID)}</strong><br>${escapeHTML(result.Name)}</td><td class="${result.Passed ? "pass" : "fail"}">${result.Passed ? (result.CriticalPass ? "临界合格" : "合格") : escapeHTML(result.FailureCategory || "失败")}</td><td>${escapeHTML(result.Detail)}</td><td>${result.EvidenceWindow?.length ? `${result.MarginMS}ms<br>${(result.LimitUsageRatio*100).toFixed(1)}%` : "—"}</td><td>${(result.CandidateWindow || []).map(escapeHTML).join("<br>") || "—"}</td></tr>`).join("")}</tbody></table></div>` : "<p>当前筛选下无规则结果</p>";
}
function failedResults() { return (current.Evaluations || []).filter(result => !result.Passed); }
function latestDeviation(ruleID) { return [...(current.Deviations || [])].reverse().find(deviation => deviation.RuleID === ruleID && deviation.Status === "open"); }
function renderDeviations() {
  const failed = failedResults();
  $("#correction-form").classList.toggle("hidden", current.Status !== "evaluation_failed");
  $("#deviation-editor").innerHTML = failed.map(result => {
    const existing = latestDeviation(result.RuleID);
    const scope = failed.map(candidate => `<label class="check-label"><input type="checkbox" data-k="scope" value="${escapeHTML(candidate.RuleID)}" ${candidate.RuleID === result.RuleID ? "checked" : ""}>${escapeHTML(candidate.RuleID)}</label>`).join("");
    return `<div class="deviation-card" data-rule="${escapeHTML(result.RuleID)}" data-deviation="${escapeHTML(existing?.DeviationID || "")}"><strong>${escapeHTML(result.RuleID)} · ${escapeHTML(result.Detail)}</strong><label>根本原因<input data-k="cause" required value=""></label><label>纠正措施<input data-k="action" required value=""></label><fieldset><legend>复演范围</legend>${scope}</fieldset></div>`;
  }).join("");
  $("#deviation-view").innerHTML = (current.Deviations || []).map(deviation => `<div class="deviation-card"><strong>${escapeHTML(deviation.RuleID)} <span class="${deviation.Status === "closed" ? "pass" : "fail"}">${escapeHTML(deviation.Status)}</span></strong><span>当前原因：${escapeHTML(deviation.RootCause)}</span><span>当前措施：${escapeHTML(deviation.CorrectiveAction)}</span><span>关闭轮次：${escapeHTML(deviation.ClosedByRunID || "—")}</span><div class="attempts">${(deviation.Attempts || []).map(attempt => `<p>第 ${attempt.attempt} 次 · ${escapeHTML(attempt.root_cause)} · ${escapeHTML(attempt.corrective_action)} · 轮次 ${escapeHTML(attempt.reperformance_run_id || "待复演")}${attempt.reperformance_failure ? ` · <span class="fail">${escapeHTML(attempt.reperformance_failure)}</span>` : ""}</p>`).join("")}</div></div>`).join("");
}
function allowedTargets() {
  const ids = new Set();
  (current.Deviations || []).filter(deviation => deviation.Status === "open").forEach(deviation => (deviation.ScopeRuleIDs || split(deviation.ReperformanceScope)).forEach(id => ids.add(id)));
  return [...ids].sort();
}
function renderReadiness(readiness) {
  $("#readiness-view").innerHTML = `<ul class="checklist">${readiness.items.map(item => `<li class="${item.passed ? "pass" : "fail"}">${item.passed ? "通过" : "未通过"} · ${escapeHTML(item.label)}<small>${escapeHTML(item.detail)}</small></li>`).join("")}</ul>${readiness.duty_conflicts.map(conflict => `<p class="fail">${escapeHTML(conflict.detail)}</p>`).join("")}<small>${escapeHTML(readiness.checklist_digest)}</small>`;
}
function renderCertificate() {
  const certificate = current.Certificate;
  $("#review-form").classList.toggle("hidden", current.Status !== "review_pending");
  if (current.Status === "review_pending") loadReadiness();
  if (!certificate) { $("#certificate-view").className = "certificate-empty"; $("#certificate-view").textContent = current.Status === "rejected" ? "案件已拒绝，无资格证书" : "尚无资格证书"; return; }
  $("#certificate-view").className = "certificate";
  $("#certificate-view").innerHTML = `<strong>${escapeHTML(certificate.CertificateID)}</strong><p>批准人 ${escapeHTML(certificate.ApprovedBy)} · ${new Date(certificate.IssuedAt).toLocaleString("zh-CN")}</p><small>${escapeHTML(certificate.CertificateDigest)}</small>`;
}
function renderVerification(verification) {
  $("#verify-view").innerHTML = `<p class="${verification.valid ? "pass" : "fail"}">${escapeHTML(verification.message)}</p>${verification.first_invalid_frame ? `<p class="fail">首个异常帧 ${verification.first_invalid_frame} · ${escapeHTML(verification.audit_failure_category)}</p>` : ""}<ul class="checklist">${verification.checks.map(check => `<li class="${check.valid ? "pass" : "fail"}">${check.valid ? "通过" : "失败"} · ${escapeHTML(check.label)}<small>${escapeHTML(check.message)}</small></li>`).join("")}</ul><details><summary>证据来源 ${verification.evidence_sources.length} 项</summary>${verification.evidence_sources.map(trace => `<div class="evidence-trace"><strong>${escapeHTML(trace.evidence.Kind)} · ${escapeHTML(trace.evidence.URI)}</strong><small>${escapeHTML(trace.evidence.SHA256)}</small>${trace.orphaned ? '<span class="fail">孤立引用</span>' : ""}${trace.missing ? '<span class="fail">清单缺失</span>' : ""}${trace.origins.map(origin => `<p>${escapeHTML(origin.run_id)} · ${escapeHTML(origin.device_id)} · ${escapeHTML(origin.event_at)} · 规则 ${escapeHTML(origin.rule_ids.join(", ") || "无")}</p>`).join("")}</div>`).join("")}</details>`;
}

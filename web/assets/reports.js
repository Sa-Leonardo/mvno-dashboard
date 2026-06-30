const state = {
  key: localStorage.getItem("mvnodashboard.adminKey") || "",
  avgActiveRevenue: Number.parseFloat(localStorage.getItem("mvnodashboard.avgActiveRevenue") || "0") || 0,
  items: [],
  reportLimit: 300,
};

const REPORT_PAGE_SIZE = 300;
const statusColors = {
  available: "#a855f7",
  active: "#22c55e",
  cancelled: "#f87171",
  blocked: "#f59e0b",
  other: "#94a3b8",
};

const cnpjNames = {
  "58420964000179": "MOV Fit",
  "58420964000330": "MOV Fit CT",
  "58420964000500": "MOV Fit CT",
  "15070244000118": "MOV Fibra",
};

const planPrices = [
  { key: "mov_tim_5gb", label: "MOV 5GB", network: "TIM", gb: 5, price: 34.97, patterns: [/mov\s*5\s*gb/i, /markup\s*5\s*gb/i, /plano\s*5\s*gb/i] },
  { key: "mov_tim_8gb", label: "MOV 8GB", network: "TIM", gb: 8, price: 44.97, patterns: [/mov\s*8\s*gb/i, /markup\s*8\s*gb/i, /plano\s*8\s*gb/i] },
  { key: "mov_tim_12gb", label: "MOV 12GB", network: "TIM", gb: 12, price: 54.97, patterns: [/mov\s*12\s*gb/i, /markup\s*12\s*gb/i, /plano\s*12\s*gb/i] },
  { key: "mov_tim_22gb", label: "MOV 22GB", network: "TIM", gb: 22, price: 64.97, patterns: [/mov\s*22\s*gb/i, /markup\s*22\s*gb/i, /plano\s*22\s*gb/i] },
  { key: "mov_vivo_5gb", label: "MOV VIP 5GB", network: "VIVO", gb: 8, price: 44.97, patterns: [/vip\s*5\s*gb/i, /vivo\s*5\s*gb/i] },
  { key: "mov_vivo_10gb", label: "MOV VIP 10GB", network: "VIVO", gb: 15, price: 61.97, patterns: [/vip\s*10\s*gb/i, /vivo\s*10\s*gb/i] },
  { key: "mov_vivo_15gb", label: "MOV VIP 15GB", network: "VIVO", gb: 20, price: 69.97, patterns: [/vip\s*15\s*gb/i, /vivo\s*15\s*gb/i] },
  { key: "mov_vivo_25gb", label: "MOV VIP 25GB", network: "VIVO", gb: 30, price: 91.97, patterns: [/vip\s*25\s*gb/i, /vivo\s*25\s*gb/i] },
  { key: "combo_mov_19gb", label: "COMBO MOV 19GB", network: "TIM", gb: 19, price: 79.97, patterns: [/combo\s*mov\s*19\s*gb/i, /combo.*19\s*gb/i] },
  { key: "combo_mov_30gb", label: "COMBO MOV 30GB", network: "TIM", gb: 30, price: 89.97, patterns: [/combo\s*mov\s*30\s*gb/i, /combo.*30\s*gb/i] },
  { key: "combo_mov_40gb", label: "COMBO MOV 40GB", network: "TIM", gb: 40, price: 99.97, patterns: [/combo\s*mov\s*40\s*gb/i, /combo.*40\s*gb/i] },
  { key: "combo_mov_45gb", label: "COMBO MOV 45GB", network: "TIM", gb: 45, price: 109.00, patterns: [/combo\s*mov\s*45\s*gb/i, /combo.*45\s*gb/i] },
];

const els = {
  adminKey: document.querySelector("#adminKey"),
  saveKeyBtn: document.querySelector("#saveKeyBtn"),
  refreshBtn: document.querySelector("#refreshBtn"),
  syncSubscribersBtn: document.querySelector("#syncSubscribersBtn"),
  syncStockBtn: document.querySelector("#syncStockBtn"),
  syncLastRechargeBtn: document.querySelector("#syncLastRechargeBtn"),
  avgActiveRevenue: document.querySelector("#avgActiveRevenue"),
  saveFinanceBtn: document.querySelector("#saveFinanceBtn"),
  statusMessage: document.querySelector("#statusMessage"),
  metricTotal: document.querySelector("#metricTotal"),
  metricAvailable: document.querySelector("#metricAvailable"),
  metricActive: document.querySelector("#metricActive"),
  metricRevenue: document.querySelector("#metricRevenue"),
  metricCancelled: document.querySelector("#metricCancelled"),
  metricBlocked: document.querySelector("#metricBlocked"),
  metricOther: document.querySelector("#metricOther"),
  metricAvailableEsim: document.querySelector("#metricAvailableEsim"),
  metricAvailablePhysical: document.querySelector("#metricAvailablePhysical"),
  metricStatusTypes: document.querySelector("#metricStatusTypes"),
  metricNoRecharge: document.querySelector("#metricNoRecharge"),
  metricDue: document.querySelector("#metricDue"),
  movSummaryBody: document.querySelector("#movSummaryBody"),
  financeSummaryBody: document.querySelector("#financeSummaryBody"),
  companySummaryBody: document.querySelector("#companySummaryBody"),
  statusDonut: document.querySelector("#statusDonut"),
  statusLegend: document.querySelector("#statusLegend"),
  reportFilter: document.querySelector("#reportFilter"),
  statusFilter: document.querySelector("#statusFilter"),
  reportCount: document.querySelector("#reportCount"),
  reportBody: document.querySelector("#reportBody"),
  loadMoreBtn: document.querySelector("#loadMoreBtn"),
  confirmModal: document.querySelector("#confirmModal"),
  confirmTitle: document.querySelector("#confirmTitle"),
  confirmMessage: document.querySelector("#confirmMessage"),
  confirmCancelBtn: document.querySelector("#confirmCancelBtn"),
  confirmOkBtn: document.querySelector("#confirmOkBtn"),
};

els.adminKey.value = state.key;
els.avgActiveRevenue.value = state.avgActiveRevenue ? String(state.avgActiveRevenue) : "";
els.confirmModal.hidden = true;

els.saveKeyBtn.addEventListener("click", () => {
  state.key = els.adminKey.value.trim();
  localStorage.setItem("mvnodashboard.adminKey", state.key);
  showMessage("Chave salva no navegador.");
});

els.refreshBtn.addEventListener("click", () => refreshReport());
els.syncSubscribersBtn.addEventListener("click", () => confirmAndRun(
  "Sincronizar toda a base de assinantes agora? Isso salva todos os contratos com ICCID retornados pela plataforma.",
  "Sincronizando base...",
  () => api("/sync/assinantes", { method: "POST" }),
));
els.syncStockBtn.addEventListener("click", () => confirmAndRun(
  "Sincronizar estoque de SIM Cards agora? Isso atualiza status real de estoque, operadora e eSIM quando a API retornar esses campos.",
  "Sincronizando estoque...",
  () => api("/sync/estoque", { method: "POST" }),
));
els.syncLastRechargeBtn.addEventListener("click", () => confirmAndRun(
  "Atualizar a ultima recarga para os ICCIDs da base? Essa consulta usa a API externa e pode levar alguns minutos.",
  "Atualizando ultima recarga...",
  () => api("/sync/ultima-recarga", { method: "POST" }),
));
els.saveFinanceBtn.addEventListener("click", () => {
  state.avgActiveRevenue = Number.parseFloat(els.avgActiveRevenue.value.replace(",", ".")) || 0;
  localStorage.setItem("mvnodashboard.avgActiveRevenue", String(state.avgActiveRevenue));
  renderAll();
  showMessage("Premissa financeira salva. Receita estimada recalculada.");
});
els.reportFilter.addEventListener("input", () => {
  resetReportLimit();
  renderReport();
});
els.statusFilter.addEventListener("change", () => {
  resetReportLimit();
  renderReport();
});
els.loadMoreBtn.addEventListener("click", () => {
  state.reportLimit += REPORT_PAGE_SIZE;
  renderReport();
});

async function api(path, options = {}) {
  if (!state.key) {
    throw new Error("Informe a chave interna antes de chamar a API.");
  }
  const init = {
    method: options.method || "GET",
    headers: {
      "x-api-key": state.key,
      "Content-Type": "application/json",
    },
  };
  if (options.body) {
    init.body = JSON.stringify(options.body);
  }
  const response = await fetch(path, init);
  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    throw new Error(`${response.status} ${data.error || data.message || response.statusText}`);
  }
  return data;
}

async function refreshReport(showOk = true) {
  try {
    const response = await api("/iccids");
    state.items = response.items || [];
    state.reportLimit = REPORT_PAGE_SIZE;
    renderAll();
    if (showOk) {
      showMessage("Relatorio atualizado.");
    }
  } catch (error) {
    showMessage(error.message, true);
  }
}

async function runAction(label, fn) {
  showMessage(label);
  try {
    const result = await fn();
    showMessage(actionSummary(result));
    await resultModal("Acao concluida", actionSummary(result));
    await refreshReport(false);
  } catch (error) {
    showMessage(error.message, true);
    await resultModal("Erro na operacao", error.message, true);
  }
}

async function confirmAndRun(message, label, fn) {
  const confirmed = await confirmModal({
    title: "Confirmar acao",
    message,
    confirmText: "Confirmar",
  });
  if (!confirmed) {
    showMessage("Acao cancelada.");
    return;
  }
  runAction(label, fn);
}

function renderAll() {
  const today = startOfToday();
  const contractItems = contractRows();
  const stats = summarizeItems(state.items);
  const availableStock = state.items.filter((item) => classifyStatus(stockStatus(item)) === "available");
  els.metricTotal.textContent = state.items.length;
  els.metricAvailable.textContent = stats.available;
  els.metricActive.textContent = stats.active;
  els.metricRevenue.textContent = formatCurrency(calculateRevenue(contractItems));
  els.metricCancelled.textContent = stats.cancelled;
  els.metricBlocked.textContent = stats.blocked;
  els.metricOther.textContent = stats.other;
  els.metricAvailableEsim.textContent = availableStock.filter((item) => item.esim === true).length;
  els.metricAvailablePhysical.textContent = availableStock.filter((item) => item.esim === false).length;
  els.metricStatusTypes.textContent = statusSummary(state.items).size;
  els.metricNoRecharge.textContent = contractItems.filter((item) => !item.last_recharge_at).length;
  els.metricDue.textContent = contractItems.filter((item) => {
    const due = parseDate(item.next_recharge_due_at);
    return due && due <= today;
  }).length;
  renderMOVSummary();
  renderFinanceSummary();
  renderCompanySummary();
  renderStatusDonut();
  renderReport();
}

function renderMOVSummary() {
  const movItems = contractRows().filter((item) => Boolean(cnpjNames[item.cnpj]));
  const summary = summarizeByCNPJ(movItems);
  for (const cnpj of Object.keys(cnpjNames)) {
    if (!summary.has(cnpj)) {
      summary.set(cnpj, { cnpj, total: 0, available: 0, active: 0, blocked: 0, cancelled: 0, other: 0, items: [] });
    }
  }
  els.movSummaryBody.innerHTML = "";
  const rows = [...summary.values()].sort((a, b) => formatCNPJ(a.cnpj).localeCompare(formatCNPJ(b.cnpj)));
  for (const item of rows) {
    els.movSummaryBody.append(row([
      formatCNPJ(item.cnpj),
      item.total,
      item.available,
      item.active,
      item.cancelled,
      item.blocked,
      formatCurrency(calculateRevenue(item.items)),
    ]));
  }
}

function renderFinanceSummary() {
  const summary = new Map();
  for (const item of contractRows()) {
    if (classifyStatus(contractStatus(item)) !== "active") continue;
    const planInfo = resolvePlanInfo(item.plan_name);
    const key = planInfo.key || `unknown:${item.plan_name || "(sem plano)"}`;
    if (!summary.has(key)) {
      summary.set(key, {
        plan: planInfo.label || item.plan_name || "(sem plano)",
        network: planInfo.network || "-",
        active: 0,
        unitPrice: planInfo.price,
        estimatedGB: planInfo.gb || extractPlanGB(item.plan_name),
        recognized: planInfo.recognized,
      });
    }
    summary.get(key).active++;
  }
  els.financeSummaryBody.innerHTML = "";
  const rows = [...summary.values()].sort((a, b) => b.active - a.active || a.plan.localeCompare(b.plan));
  if (!rows.length) {
    els.financeSummaryBody.append(emptyRow(7, "Nenhum chip ativo para detalhar receita."));
    return;
  }
  for (const item of rows) {
    els.financeSummaryBody.append(row([
      item.plan,
      item.network,
      item.active,
      formatCurrency(item.unitPrice),
      formatCurrency(item.active * item.unitPrice),
      item.estimatedGB ? `${item.estimatedGB} GB` : "-",
      item.recognized ? "Sim" : "Fallback",
    ]));
  }
}

function renderCompanySummary() {
  const summary = summarizeByCNPJ(contractRows());
  els.companySummaryBody.innerHTML = "";
  const rows = [...summary.values()].sort((a, b) => formatCNPJ(a.cnpj).localeCompare(formatCNPJ(b.cnpj)));
  if (!rows.length) {
    els.companySummaryBody.append(emptyRow(7, "Nenhum contrato na base."));
    return;
  }
  for (const item of rows) {
    els.companySummaryBody.append(row([
      formatCNPJ(item.cnpj),
      item.total,
      item.available,
      item.active,
      item.blocked,
      item.cancelled,
      formatCurrency(calculateRevenue(item.items)),
    ]));
  }
}

function renderStatusDonut() {
  const summary = statusSummary(state.items);
  const rows = [...summary.entries()].sort((a, b) => b[1] - a[1]);
  const total = rows.reduce((sum, [, count]) => sum + count, 0);
  if (!rows.length || total === 0) {
    els.statusDonut.innerHTML = `<div class="muted">Sem dados</div>`;
    els.statusLegend.innerHTML = "";
    return;
  }
  let offset = 25;
  const circles = rows.map(([status, count]) => {
    const type = classifyStatus(status);
    const length = (count / total) * 100;
    const circle = `<circle r="15.915" cx="18" cy="18" fill="transparent" stroke="${statusColors[type]}" stroke-width="4" stroke-dasharray="${length} ${100 - length}" stroke-dashoffset="${offset}"></circle>`;
    offset -= length;
    return circle;
  }).join("");
  els.statusDonut.innerHTML = `
    <svg viewBox="0 0 36 36" role="img" aria-label="Distribuicao por status">
      ${circles}
    </svg>
  `;
  els.statusLegend.innerHTML = rows.map(([status, count]) => {
    const type = classifyStatus(status);
    return `
      <div class="legend-item">
        <span class="legend-dot" style="background:${statusColors[type]}"></span>
        <span>${escapeHTML(status)}</span>
        <strong>${count}</strong>
      </div>
    `;
  }).join("");
}

function renderReport() {
  const search = els.reportFilter.value.trim().toLowerCase();
  const status = els.statusFilter.value;
  const filtered = state.items.filter((item) => {
    const haystack = [
      item.sim_card,
      item.phone_number,
      item.cnpj,
      formatCNPJ(item.cnpj),
      item.subscriber_name,
      item.contract_number,
      item.contract_status,
      item.stock_status,
      item.operator,
      esimLabel(item.esim),
      item.plan_name,
    ].join(" ").toLowerCase();
    return (!search || haystack.includes(search)) && (!status || effectiveStatus(item) === status);
  });
  const visible = filtered.slice(0, state.reportLimit);
  els.reportBody.innerHTML = "";
  els.reportCount.textContent = `Mostrando ${visible.length} de ${filtered.length} ICCIDs. Use os filtros para refinar a consulta.`;
  els.loadMoreBtn.hidden = visible.length >= filtered.length;
  if (!visible.length) {
    els.reportBody.append(emptyRow(13, "Nenhum contrato encontrado."));
    return;
  }
  for (const item of visible) {
    els.reportBody.append(row([
      item.sim_card,
      item.phone_number || "-",
      formatCNPJ(item.cnpj),
      item.subscriber_name || "-",
      item.contract_number || "-",
      item.contract_status ? badge(item.contract_status) : "-",
      item.stock_status ? badge(item.stock_status) : "-",
      item.operator || "-",
      esimLabel(item.esim),
      item.plan_name || "-",
      formatDate(item.last_recharge_at),
      formatDate(item.next_recharge_due_at),
      formatDateTime(item.updated_at),
    ]));
  }
}

function confirmModal({ title, message, confirmText = "Confirmar", danger = false }) {
  return new Promise((resolve) => {
    els.confirmTitle.textContent = title;
    els.confirmMessage.textContent = message;
    els.confirmOkBtn.textContent = confirmText;
    els.confirmOkBtn.classList.toggle("danger", danger);
    els.confirmModal.hidden = false;

    const cleanup = (value) => {
      els.confirmModal.hidden = true;
      els.confirmOkBtn.classList.remove("danger");
      els.confirmOkBtn.removeEventListener("click", onConfirm);
      els.confirmCancelBtn.removeEventListener("click", onCancel);
      els.confirmModal.removeEventListener("click", onBackdrop);
      document.removeEventListener("keydown", onKeydown);
      resolve(value);
    };
    const onConfirm = () => cleanup(true);
    const onCancel = () => cleanup(false);
    const onBackdrop = (event) => {
      if (event.target === els.confirmModal) cleanup(false);
    };
    const onKeydown = (event) => {
      if (event.key === "Escape") cleanup(false);
    };

    els.confirmOkBtn.addEventListener("click", onConfirm);
    els.confirmCancelBtn.addEventListener("click", onCancel);
    els.confirmModal.addEventListener("click", onBackdrop);
    document.addEventListener("keydown", onKeydown);
    els.confirmCancelBtn.focus();
  });
}

function resultModal(title, message, danger = false) {
  return new Promise((resolve) => {
    els.confirmTitle.textContent = title;
    els.confirmMessage.textContent = message;
    els.confirmOkBtn.textContent = "Ok";
    els.confirmOkBtn.classList.toggle("danger", danger);
    els.confirmCancelBtn.hidden = true;
    els.confirmModal.hidden = false;

    const cleanup = () => {
      els.confirmModal.hidden = true;
      els.confirmCancelBtn.hidden = false;
      els.confirmOkBtn.classList.remove("danger");
      els.confirmOkBtn.removeEventListener("click", onConfirm);
      els.confirmModal.removeEventListener("click", onBackdrop);
      document.removeEventListener("keydown", onKeydown);
      resolve();
    };
    const onConfirm = () => cleanup();
    const onBackdrop = (event) => {
      if (event.target === els.confirmModal) cleanup();
    };
    const onKeydown = (event) => {
      if (event.key === "Escape" || event.key === "Enter") cleanup();
    };

    els.confirmOkBtn.addEventListener("click", onConfirm);
    els.confirmModal.addEventListener("click", onBackdrop);
    document.addEventListener("keydown", onKeydown);
    els.confirmOkBtn.focus();
  });
}

function row(values) {
  const tr = document.createElement("tr");
  for (const value of values) {
    const td = document.createElement("td");
    if (value instanceof HTMLElement) {
      td.append(value);
    } else {
      const text = String(value ?? "-");
      td.textContent = text;
      if (text === "-") {
        td.className = "empty-cell";
      }
    }
    tr.append(td);
  }
  return tr;
}

function emptyRow(cols, message) {
  const tr = document.createElement("tr");
  const td = document.createElement("td");
  td.colSpan = cols;
  td.className = "muted";
  td.textContent = message;
  tr.append(td);
  return tr;
}

function badge(status) {
  const value = status || "-";
  const span = document.createElement("span");
  span.className = "badge";
  const type = classifyStatus(value);
  if (type === "active" || value === "success") span.classList.add("ok");
  if (type === "blocked" || type === "available" || value === "pending") span.classList.add("warn");
  if (type === "cancelled" || value === "failed") span.classList.add("danger");
  span.textContent = value;
  return span;
}

function resetReportLimit() {
  state.reportLimit = REPORT_PAGE_SIZE;
}

function formatCNPJ(value) {
  const raw = String(value || "");
  const name = cnpjNames[raw];
  return name ? `${name} - ${raw}` : raw || "-";
}

function normalizeStatus(value) {
  return String(value || "").trim().toUpperCase();
}

function effectiveStatus(item) {
  return item.contract_status || item.stock_status || "";
}

function stockStatus(item) {
  return item.stock_status || "";
}

function contractStatus(item) {
  return item.contract_status || "";
}

function hasStockData(item) {
  return Boolean(item.stock_sync_at || item.stock_status);
}

function hasContractData(item) {
  return Boolean(item.cnpj || item.contract_number || item.phone_number || item.contract_status || item.plan_name);
}

function stockRows() {
  return state.items.filter(hasStockData);
}

function contractRows() {
  return state.items.filter(hasContractData);
}

function esimLabel(value) {
  if (value === true) return "Sim";
  if (value === false) return "Nao";
  return "-";
}

function classifyStatus(value) {
  const status = normalizeStatus(value).normalize("NFD").replace(/[\u0300-\u036f]/g, "");
  if (status === "EM USO" || status === "ATIVO" || status === "ACTIVE") return "active";
  if (status.includes("CANCEL")) return "cancelled";
  if (status.includes("BLOQUE")) return "blocked";
  if (status.includes("DISPON") || status.includes("LIVRE") || status.includes("ESTOQUE") || status.includes("AVAILABLE")) return "available";
  return "other";
}

function summarizeItems(items) {
  const stats = { total: items.length, available: 0, active: 0, cancelled: 0, blocked: 0, other: 0 };
  for (const item of items) {
    const type = classifyStatus(effectiveStatus(item));
    if (type === "available") stats.available++;
    else if (type === "active") stats.active++;
    else if (type === "cancelled") stats.cancelled++;
    else if (type === "blocked") stats.blocked++;
    else stats.other++;
  }
  return stats;
}

function statusSummary(items) {
  const summary = new Map();
  for (const item of items) {
    const status = normalizeStatus(effectiveStatus(item)) || "(vazio)";
    summary.set(status, (summary.get(status) || 0) + 1);
  }
  return summary;
}

function summarizeByCNPJ(items) {
  const summary = new Map();
  for (const item of items) {
    const cnpj = item.cnpj || "-";
    if (!summary.has(cnpj)) {
      summary.set(cnpj, { cnpj, total: 0, available: 0, active: 0, blocked: 0, cancelled: 0, other: 0, items: [] });
    }
    const row = summary.get(cnpj);
    row.items.push(item);
    row.total++;
    const type = classifyStatus(contractStatus(item));
    if (type === "available") row.available++;
    else if (type === "active") row.active++;
    else if (type === "cancelled") row.cancelled++;
    else if (type === "blocked") row.blocked++;
    else row.other++;
  }
  return summary;
}

function calculateRevenue(items) {
  return items.reduce((total, item) => {
    if (classifyStatus(contractStatus(item)) !== "active") return total;
    return total + resolvePlanInfo(item.plan_name).price;
  }, 0);
}

function resolvePlanInfo(planName) {
  const normalized = normalizeText(planName);
  let candidates = planPrices;
  if (/vivo/i.test(normalized)) {
    candidates = planPrices.filter((plan) => plan.network === "VIVO");
  } else if (/combo/i.test(normalized)) {
    candidates = planPrices.filter((plan) => plan.key.startsWith("combo_"));
  }
  const found = candidates.find((plan) => plan.patterns.some((pattern) => pattern.test(normalized)));
  if (found) {
    return { ...found, recognized: true };
  }
  return {
    key: null,
    label: planName || "(sem plano)",
    network: "-",
    gb: extractPlanGB(planName),
    price: state.avgActiveRevenue,
    recognized: false,
  };
}

function extractPlanGB(planName) {
  const match = String(planName || "").match(/(\d+(?:[,.]\d+)?)\s*gb/i);
  if (!match) return null;
  return Number.parseFloat(match[1].replace(",", "."));
}

function normalizeText(value) {
  return String(value || "").normalize("NFD").replace(/[\u0300-\u036f]/g, "");
}

function formatCurrency(value) {
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
  }).format(Number.isFinite(value) ? value : 0);
}

function formatDate(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value).slice(0, 10);
  return new Intl.DateTimeFormat("pt-BR").format(date);
}

function formatDateTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(date);
}

function parseDate(value) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function startOfToday() {
  const date = new Date();
  date.setHours(0, 0, 0, 0);
  return date;
}

function showMessage(message, isError = false) {
  els.statusMessage.hidden = false;
  els.statusMessage.classList.toggle("error", isError);
  els.statusMessage.textContent = message;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function actionSummary(result) {
  if (result.saved !== undefined) {
    if (result.total_stock_items !== undefined) {
      return `Estoque sincronizado. Salvos: ${result.saved}. eSIM: ${result.esim_count ?? 0}. Ignorados: ${result.skipped ?? 0}.`;
    }
    return `Base sincronizada. Salvos: ${result.saved}. Permitidos: ${result.saved_allowed ?? "-"}. Informativos: ${result.saved_non_allowed ?? "-"}. Ignorados: ${result.skipped ?? "-"}.`;
  }
  if (result.updated !== undefined) {
    return `Ultima recarga sincronizada. Atualizados: ${result.updated}. Falhas: ${result.failed}.`;
  }
  return "Acao concluida.";
}

if (state.key) {
  refreshReport(false);
}

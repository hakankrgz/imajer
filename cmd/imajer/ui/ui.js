let token = "";
let defaults = {};
let lastLogCount = -1;

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];

async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (options.body) headers["Content-Type"] = "application/json";
  if (token) headers["X-Imajer-Token"] = token;
  const response = await fetch(path, { ...options, headers });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `İşlem reddedildi (${response.status})`);
  return data;
}

function toast(message) {
  const element = $("#toast");
  element.textContent = message;
  element.classList.remove("hidden");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => element.classList.add("hidden"), 4200);
}

function switchTab(name) {
  $$(".tab").forEach(item => item.classList.toggle("active", item.dataset.tab === name));
  $$(".panel").forEach(item => item.classList.toggle("active", item.id === `panel-${name}`));
}

function updateConditionalFields() {
  const transport = $('input[name="transport"]:checked')?.value || "local";
  const profile = $('input[name="profile"]:checked')?.value || "disk";
  $$(".remote-fields").forEach(el => el.classList.toggle("hidden", transport === "local"));
  $$(".ssh-fields").forEach(el => el.classList.toggle("hidden", transport !== "ssh"));
  $$(".winrm-fields").forEach(el => el.classList.toggle("hidden", transport !== "winrm"));
  $$(".disk-fields").forEach(el => el.classList.toggle("hidden", profile === "ram"));
  $$(".ram-fields").forEach(el => el.classList.toggle("hidden", profile === "disk"));
  const port = $('[name="port"]');
  if (transport === "ssh" && (!port.value || port.value === "5986")) port.value = "22";
  if (transport === "winrm" && (!port.value || port.value === "22")) port.value = "5986";
}

function newJobPayload() {
  const form = $("#newJobForm");
  if (!form.reportValidity()) throw new Error("Lütfen zorunlu alanları doldurun");
  const data = new FormData(form);
  return {
    case_id: data.get("case_id") || "",
    evidence_id: data.get("evidence_id") || "",
    examiner: data.get("examiner") || "",
    organization: data.get("organization") || "",
    authority_ref: data.get("authority_ref") || "",
    notes: data.get("notes") || "",
    authorized: data.get("authorized") === "on",
    transport: data.get("transport") || "local",
    host: data.get("host") || "",
    port: data.get("port") || "",
    user: data.get("user") || "",
    auth: data.get("auth") || "",
    private_key: data.get("private_key") || "",
    known_hosts: data.get("known_hosts") || "",
    ca_file: data.get("ca_file") || "",
    profile: data.get("profile") || "disk",
    disk_path: data.get("disk_path") || "",
    disk_id: data.get("disk_id") || "",
    disk_model: data.get("disk_model") || "",
    disk_size: data.get("disk_size") || "",
    sector_size: data.get("sector_size") || "",
    disk_provider: data.get("disk_provider") || "native",
    ram_provider: data.get("ram_provider") || "auto",
    ram_tool_name: data.get("ram_tool_name") || "",
    ram_tool_local: data.get("ram_tool_local") || "",
    output_directory: data.get("output_directory") || "",
    signing_key: data.get("signing_key") || "",
    agent_local: data.get("agent_local") || "",
    agent_remote: data.get("agent_remote") || "",
    tool_manifest: data.get("tool_manifest") || "",
    trust_public_key: data.get("trust_public_key") || ""
  };
}

async function run(request) {
  try {
    const result = await api("/api/run", { method: "POST", body: JSON.stringify(request) });
    if (result.job_path) $("#existingJob").value = result.job_path;
    if (result.case_dir) $("#verifyCaseDir").value = result.case_dir;
    toast(result.message || "İşlem başlatıldı");
    await pollStatus();
  } catch (error) {
    toast(error.message);
  }
}

function formatAction(action) {
  return {
    discover: "Hedef kontrolü", acquire: "İmaj alma", resume: "Devam",
    verify: "Kanıt doğrulama", report: "Rapor yenileme", cleanup: "Temizlik"
  }[action] || action || "—";
}

async function pollStatus() {
  try {
    const status = await api("/api/status");
    $("#statusTitle").textContent = status.message || "Hazır";
    $("#statusAction").textContent = formatAction(status.action);
    $("#statusStarted").textContent = status.started_at ? new Date(status.started_at).toLocaleString("tr-TR") : "—";
    $("#statusCase").textContent = status.case_dir || "—";
    $("#statusCase").title = status.case_dir || "";
    const dot = $("#statusDot");
    dot.className = "status-dot";
    if (status.running) dot.classList.add("running");
    else if (status.finished_at) dot.classList.add(status.success ? "success" : "failed");
    $("#cancelButton").classList.toggle("hidden", !status.running);
    $$("button").forEach(button => {
      if (button.id !== "cancelButton" && !button.classList.contains("tab")) button.disabled = status.running;
    });
    if ((status.logs || []).length !== lastLogCount) {
      const consoleElement = $("#console");
      consoleElement.textContent = (status.logs || []).join("\n") || "Henüz kayıt yok.";
      consoleElement.scrollTop = consoleElement.scrollHeight;
      lastLogCount = (status.logs || []).length;
    }
  } catch (error) {
    $("#statusTitle").textContent = "Arayüz bağlantısı kesildi";
  }
}

async function initialise() {
  defaults = await api("/api/config");
  token = defaults.token;
  $("#existingJob").value = defaults.demo_job;
  $("#verifyCaseDir").value = defaults.demo_case_dir;
  $("#verifyPublicKey").value = defaults.demo_public_key;
  $("#agentLocal").value = defaults.default_agent;
  $('[name="output_directory"]').value = `${defaults.working_dir}/evidence`;
  updateConditionalFields();
  await pollStatus();
  setInterval(pollStatus, 700);
}

$$(".tab").forEach(button => button.addEventListener("click", () => switchTab(button.dataset.tab)));
$$('input[name="transport"], input[name="profile"]').forEach(input => input.addEventListener("change", updateConditionalFields));

$$(".existing-action").forEach(button => button.addEventListener("click", () => run({
  action: button.dataset.action,
  job_path: $("#existingJob").value,
  password: $("#existingPassword").value
})));

$$(".new-action").forEach(button => button.addEventListener("click", () => {
  try {
    const form = $("#newJobForm");
    run({
      action: button.dataset.action,
      password: form.elements.password.value,
      job: newJobPayload()
    });
  } catch (error) {
    toast(error.message);
  }
}));

$("#verifyButton").addEventListener("click", () => run({
  action: "verify",
  case_dir: $("#verifyCaseDir").value,
  public_key: $("#verifyPublicKey").value
}));

$("#demoDiscover").addEventListener("click", () => run({ action: "discover", job_path: defaults.demo_job }));
$("#demoAcquire").addEventListener("click", () => run({ action: "acquire", job_path: defaults.demo_job }));
$("#demoVerify").addEventListener("click", () => run({
  action: "verify", case_dir: defaults.demo_case_dir, public_key: defaults.demo_public_key
}));

$("#cancelButton").addEventListener("click", async () => {
  try {
    const result = await api("/api/cancel", { method: "POST", body: "{}" });
    toast(result.message);
  } catch (error) {
    toast(error.message);
  }
});

initialise().catch(error => toast(`Arayüz başlatılamadı: ${error.message}`));

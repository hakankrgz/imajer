let token = "";
let defaults = {};
let lastLogCount = -1;
let lastTransport = "";
let inventoryDisks = [];
let inventoryReady = false;
let pendingHostKeyFingerprint = "";
let lastFinishedAt = "";

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
  const needsDisk = profile !== "ram";
  $$(".remote-fields").forEach(el => el.classList.toggle("hidden", transport === "local"));
  $$(".ssh-fields").forEach(el => el.classList.toggle("hidden", transport !== "ssh"));
  $$(".winrm-fields").forEach(el => el.classList.toggle("hidden", transport !== "winrm"));
  $$(".disk-fields").forEach(el => el.classList.toggle("hidden", !needsDisk));
  $$(".ram-fields").forEach(el => el.classList.toggle("hidden", profile === "disk"));
  $$(".remote-disk-picker").forEach(el => el.classList.toggle("hidden", transport === "local"));
  $$(".local-disk-picker").forEach(el => el.classList.toggle("hidden", transport !== "local"));
  $("#diskSelect").required = false;
  $("#localSourcePath").required = transport === "local" && needsDisk;
  const port = $('[name="port"]');
  if (transport === "ssh" && (!port.value || port.value === "5986")) port.value = "22";
  if (transport === "winrm" && (!port.value || port.value === "22")) port.value = "5986";
  if (transport !== lastTransport) clearInventory();
  if (transport !== lastTransport && defaults.agents) {
    const recommended = transport === "ssh"
      ? defaults.agents.linux_amd64
      : transport === "winrm"
        ? defaults.agents.windows_amd64
        : defaults.agents.local;
    if (recommended) $("#agentLocal").value = recommended;
    lastTransport = transport;
  }
}

function clearInventory() {
  inventoryReady = false;
  inventoryDisks = [];
  pendingHostKeyFingerprint = "";
  $("#hostKeyReview").classList.add("hidden");
  $("#hostKeyFingerprint").textContent = "";
  const select = $("#diskSelect");
  select.innerHTML = '<option value="">Önce “Bağlan ve diskleri getir” düğmesine basın</option>';
  select.value = "";
  clearSelectedDisk();
  $("#targetScanStatus").textContent = "Bağlantı bilgilerini girip bu düğmeye basın.";
}

function clearSelectedDisk() {
  const summary = $("#selectedDiskSummary");
  summary.classList.add("hidden");
  summary.replaceChildren();
  for (const name of ["disk_path", "disk_id", "disk_model", "disk_size"]) {
    $(`[name="${name}"]`).value = "";
  }
}

function formatBytes(value) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let number = Number(value) || 0;
  let unit = 0;
  while (number >= 1024 && unit < units.length - 1) {
    number /= 1024;
    unit += 1;
  }
  return `${number.toLocaleString("tr-TR", { maximumFractionDigits: 2 })} ${units[unit]}`;
}

function selectDisk(index) {
  const disk = inventoryDisks[index];
  if (!disk) {
    clearSelectedDisk();
    return;
  }
  $('[name="disk_path"]').value = disk.path || "";
  $('[name="disk_id"]').value = disk.id || disk.path || "";
  $('[name="disk_model"]').value = disk.model || "";
  $('[name="disk_size"]').value = String(disk.size || "");
  $('[name="sector_size"]').value = String(disk.sector_size || 512);
  const summary = $("#selectedDiskSummary");
  const title = document.createElement("b");
  title.textContent = `Seçilen disk: ${disk.model || "Model bilgisi yok"}`;
  const details = document.createElement("span");
  details.textContent =
    `${disk.path} · ${formatBytes(disk.size)} · ID: ${disk.serial || disk.id || "yok"} · Sektör: ${disk.sector_size || 512} B`;
  const children = [title, details];
  if (disk.mounted) {
    const warning = document.createElement("em");
    const mounts = (disk.mountpoints || []).join(", ");
    warning.textContent = `Dikkat: Bu disk çalışan sistemde bağlı${mounts ? ` (${mounts})` : ""}. İmaj alınırken içeriği değişebilir.`;
    children.push(warning);
  }
  if (!disk.stable_id) {
    const warning = document.createElement("em");
    warning.textContent = "Bu aygıt seri/WWN bildirmiyor; Raspberry Pi SD kartlarında normaldir. Resume kimliği yol + model + boyut + sektör ile denetlenir.";
    children.push(warning);
  }
  summary.replaceChildren(...children);
  summary.classList.remove("hidden");
}

function connectionPayload() {
  const data = new FormData($("#newJobForm"));
  return {
    transport: data.get("transport") || "local",
    host: data.get("host") || "",
    port: data.get("port") || "",
    user: data.get("user") || "",
    auth: data.get("auth") || "",
    private_key: data.get("private_key") || "",
    known_hosts: data.get("known_hosts") || "",
    ca_file: data.get("ca_file") || ""
  };
}

async function scanTarget() {
  const form = $("#newJobForm");
  const target = connectionPayload();
  if (!target.host || !target.user) {
    throw new Error("Sunucu/IP ve kullanıcı alanlarını doldurun");
  }
  if (target.transport === "ssh" && !target.known_hosts) {
    $(".ssh-fields")?.closest("details")?.setAttribute("open", "");
    throw new Error("SSH için Gelişmiş bağlantı ayarlarındaki known_hosts yolu gereklidir");
  }
  const button = $("#scanTargetButton");
  button.disabled = true;
  $("#targetScanStatus").textContent = "Bağlanılıyor ve fiziksel diskler okunuyor…";
  try {
    if (target.transport === "ssh") {
      const hostKey = await api("/api/ssh-host-key", {
        method: "POST",
        body: JSON.stringify({ job: target })
      });
      if (!hostKey.trusted) {
        pendingHostKeyFingerprint = hostKey.fingerprint;
        $("#hostKeyFingerprint").textContent =
          `${hostKey.algorithm}  ${hostKey.fingerprint}`;
        $("#hostKeyReview").classList.remove("hidden");
        $("#targetScanStatus").textContent =
          "İlk bağlantı: aşağıdaki SSH fingerprint onayınızı bekliyor.";
        return;
      }
      pendingHostKeyFingerprint = "";
      $("#hostKeyReview").classList.add("hidden");
    }
    const inventory = await api("/api/inventory", {
      method: "POST",
      body: JSON.stringify({
        job: target,
        password: form.elements.password.value
      })
    });
    inventoryReady = true;
    inventoryDisks = inventory.disks || [];
    const select = $("#diskSelect");
    select.innerHTML = '<option value="">Bir fiziksel disk seçin</option>';
    inventoryDisks.forEach((disk, index) => {
      const option = document.createElement("option");
      option.value = String(index);
      option.textContent = `${disk.mounted ? "[BAĞLI/SİSTEM] " : ""}${disk.model || disk.path} — ${formatBytes(disk.size)} — ID: ${disk.serial || disk.id || "yok"} — ${disk.path}`;
      select.appendChild(option);
    });
    if (inventory.agent_local) $("#agentLocal").value = inventory.agent_local;
    const privilege = inventory.privilege === "passwordless_sudo"
      ? "parolasız sudo hazır"
      : inventory.privilege === "root"
        ? "root hazır"
        : inventory.admin
          ? "yönetici hazır"
          : "root/parolasız sudo yok";
    $("#targetScanStatus").textContent =
      `${inventory.hostname || target.host} · ${inventory.os}/${inventory.arch} · ${inventoryDisks.length} disk · ${privilege}`;
    if ((inventory.warnings || []).length) toast(inventory.warnings.join(" · "));
    const profile = $('input[name="profile"]:checked')?.value || "disk";
    if (!inventoryDisks.length && profile !== "ram") {
      throw new Error("Sunucuda seçilebilir fiziksel disk bulunamadı");
    }
    toast(profile === "ram"
      ? "Sunucu tanındı. RAM edinimi için hedef hazır."
      : "Sunucu tanındı. Şimdi listeden imajı alınacak diski seçin.");
  } finally {
    button.disabled = false;
  }
}

function newJobPayload() {
  const form = $("#newJobForm");
  const transport = $('input[name="transport"]:checked')?.value || "local";
  const profile = $('input[name="profile"]:checked')?.value || "disk";
  if (transport === "local") {
    $('[name="disk_path"]').value = $("#localSourcePath").value.trim();
  } else if (!inventoryReady) {
    throw new Error("Önce “Bağlan ve diskleri getir” ile sunucuyu tanıyın");
  }
  if (transport !== "local" && profile !== "ram" && !$('[name="disk_path"]').value.trim()) {
    throw new Error("İmajı alınacak fiziksel diski listeden seçin");
  }
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

function formatDuration(startedAt, finishedAt) {
  if (!startedAt) return "—";
  const start = new Date(startedAt).getTime();
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now();
  const totalSeconds = Math.max(0, Math.floor((end - start) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours) return `${hours} sa ${minutes} dk ${seconds} sn`;
  if (minutes) return `${minutes} dk ${seconds} sn`;
  return `${seconds} sn`;
}

function resultBadge(label) {
  const element = document.createElement("span");
  element.className = "result-badge";
  element.textContent = label;
  return element;
}

function latestUsefulLog(logs) {
  for (let index = logs.length - 1; index >= 0; index -= 1) {
    const line = logs[index].trim();
    if (line && !line.startsWith("$ imajer")) return line;
  }
  return "";
}

function integrityStatusLabel(value) {
  return {
    verified_continuous: "KESİNTİSİZ DOĞRULANDI",
    chunk_verified_composite: "CHUNK DOĞRULAMALI BİLEŞİK",
    incomplete: "TAMAMLANMADI",
    failed: "BAŞARISIZ",
    running: "DEVAM EDİYOR"
  }[value] || (value || "BİLİNMİYOR").toLocaleUpperCase("tr-TR");
}

function hashComparisonRow(label, remote, local, match, remoteLabel = "Uzak SHA-256", localLabel = "Yerel SHA-256") {
  const row = document.createElement("div");
  row.className = "hash-comparison";
  const title = document.createElement("b");
  title.textContent = label;
  const state = document.createElement("span");
  state.className = match ? "hash-match" : "hash-mismatch";
  state.textContent = match ? "EŞLEŞİYOR" : "EŞLEŞMİYOR";
  const remoteName = document.createElement("small");
  remoteName.textContent = remoteLabel;
  const remoteHash = document.createElement("code");
  remoteHash.textContent = remote || "—";
  const localName = document.createElement("small");
  localName.textContent = localLabel;
  const localHash = document.createElement("code");
  localHash.textContent = local || "—";
  row.append(title, state, remoteName, remoteHash, localName, localHash);
  return row;
}

function singleHashRow(label, hash) {
  const row = document.createElement("div");
  row.className = "single-hash";
  const title = document.createElement("small");
  title.textContent = label;
  const code = document.createElement("code");
  code.textContent = hash || "—";
  row.append(title, code);
  return row;
}

function renderIntegrity(status) {
  const panel = $("#integrityPanel");
  const container = $("#integrityArtifacts");
  const summary = status.integrity;
  const artifacts = summary?.artifacts || [];
  if (!artifacts.length) {
    panel.classList.add("hidden");
    container.replaceChildren();
    return;
  }
  let allValid = true;
  const cards = artifacts.map(artifact => {
    const card = document.createElement("article");
    card.className = "integrity-artifact";
    const heading = document.createElement("div");
    heading.className = "integrity-artifact-heading";
    const name = document.createElement("b");
    name.textContent = `${artifact.kind === "ram" ? "RAM" : "Disk"} · ${artifact.artifact_id}`;
    const state = document.createElement("span");
    state.textContent = integrityStatusLabel(artifact.status);
    heading.append(name, state);
    const facts = document.createElement("p");
    facts.textContent = `${formatBytes(artifact.received_size)} / ${formatBytes(artifact.source_size)} · ${artifact.chunks} chunk · ${artifact.segments} parça · ${artifact.sessions?.length || 0} oturum`;
    card.append(heading, facts);

    if (artifact.remote_full_sha256) {
      card.append(hashComparisonRow(
        "Kesintisiz tam akış karşılaştırması",
        artifact.remote_full_sha256,
        artifact.logical_sha256,
        artifact.continuous_match,
        "Agent uzak tam akış SHA-256",
        "Yerel bağımsız yeniden okuma SHA-256"
      ));
      allValid = allValid && artifact.continuous_match;
    } else {
      const note = document.createElement("p");
      note.className = "integrity-note";
      note.textContent = artifact.resumed
        ? "Bağlantı kesilip devam edildiği için tek bir uzak tam kaynak hash’i iddia edilmez. Aşağıdaki her oturum ayrı doğrulanmıştır."
        : "Bu artifact için kesintisiz uzak tam akış hash’i yoktur; yerel birleşim ve oturum hash’leri gösterilir.";
      card.append(note);
      if (artifact.status === "failed" || artifact.status === "incomplete") allValid = false;
    }
    card.append(singleHashRow("Yerel mantıksal birleşim SHA-256", artifact.logical_sha256));
    card.append(singleHashRow("Sıralı chunk Merkle root", artifact.merkle_root));

    if (artifact.sessions?.length) {
      const sessions = document.createElement("details");
      sessions.className = "session-comparisons";
      sessions.open = artifact.sessions.length <= 3;
      const sessionsTitle = document.createElement("summary");
      sessionsTitle.textContent = `Oturum hash karşılaştırmaları (${artifact.sessions.length})`;
      sessions.append(sessionsTitle);
      artifact.sessions.forEach((session, index) => {
        sessions.append(hashComparisonRow(
          `Oturum ${index + 1} · ofset ${session.start_offset}–${session.end_offset} · ${formatBytes(session.bytes)}`,
          session.remote_sha256,
          session.local_sha256,
          session.match,
          "Agent oturum SHA-256",
          "Denetleyici oturum SHA-256"
        ));
        allValid = allValid && session.match;
      });
      card.append(sessions);
    } else if (artifact.status === "verified_continuous" || artifact.status === "chunk_verified_composite") {
      allValid = false;
    }
    return card;
  });
  container.replaceChildren(...cards);
  $("#integrityOverall").textContent = allValid ? "HASH’LER EŞLEŞİYOR" : "DİKKAT GEREKİYOR";
  $("#integrityOverall").className = allValid ? "hash-match" : "hash-mismatch";
  panel.classList.remove("hidden");
}

function renderResult(status) {
  const panel = $("#resultPanel");
  const heading = $("#resultHeading");
  const summary = $("#resultSummary");
  const icon = $("#resultIcon");
  const badges = $("#resultBadges");
  const logs = status.logs || [];
  const logText = logs.join("\n");
  renderIntegrity(status);
  badges.replaceChildren();
  panel.className = "result-panel";

  if (status.running) {
    panel.classList.add("running");
    icon.textContent = "…";
    heading.textContent = `${formatAction(status.action)} devam ediyor`;
    summary.textContent = latestUsefulLog(logs) || "İşlem başladı; canlı kayıtlar aşağıda gösteriliyor.";
    return;
  }
  if (!status.finished_at) {
    panel.classList.add("idle");
    icon.textContent = "i";
    heading.textContent = "Henüz işlem başlatılmadı";
    summary.textContent = "Bir işlem başladığında ilerleme ve sonuç burada açıklanacak.";
    return;
  }
  if (!status.success) {
    const canceled = (status.message || "").toLocaleLowerCase("tr-TR").includes("iptal");
    panel.classList.add(canceled ? "canceled" : "failed");
    icon.textContent = canceled ? "!" : "×";
    heading.textContent = canceled ? "İşlem güvenle durduruldu" : "İşlem tamamlanamadı";
    summary.textContent = status.message || latestUsefulLog(logs) || "Ayrıntılar canlı kayıtta.";
    return;
  }

  panel.classList.add("success");
  icon.textContent = "✓";
  if (status.action === "acquire" || status.action === "resume") {
    heading.textContent = "İmaj alma tamamlandı";
    summary.textContent = "Kanıt dosyaları kaydedildi. Son adım olarak “Kanıt doğrula” işlemini çalıştırın.";
    badges.append(resultBadge("İMAJ TAMAMLANDI"));
  } else if (status.action === "verify") {
    const acquisitionVerified = logText.includes("ACQUISITION_VERIFIED");
    const packageVerified = logText.includes("PACKAGE_INTEGRITY_OK");
    heading.textContent = acquisitionVerified && packageVerified
      ? "Kanıt başarıyla doğrulandı"
      : "Kanıt kontrolü tamamlandı";
    summary.textContent = acquisitionVerified && packageVerified
      ? "İmaj bütünlüğü, hash değerleri ve imzalı kanıt paketi geçerli."
      : "Kontrol tamamlandı; ayrıntılı sonucu aşağıdaki canlı kayıttan inceleyin.";
    if (acquisitionVerified) badges.append(resultBadge("İMAJ DOĞRULANDI"));
    if (packageVerified) badges.append(resultBadge("İMZA GEÇERLİ"));
  } else if (status.action === "discover") {
    heading.textContent = "Hedef kontrolü tamamlandı";
    summary.textContent = "Hedef bilgileri ve erişim koşulları kontrol edildi. İmaj alma işlemine geçebilirsiniz.";
  } else if (status.action === "report") {
    heading.textContent = "Rapor başarıyla yenilendi";
    summary.textContent = "Vaka raporu güncel işlem ve doğrulama kayıtlarıyla oluşturuldu.";
  } else if (status.action === "cleanup") {
    heading.textContent = "Temizlik tamamlandı";
    summary.textContent = "IMAJER’in hedefte oluşturduğu geçici agent ve araç izleri için cleanup tamamlandı.";
  } else {
    heading.textContent = "İşlem başarıyla tamamlandı";
    summary.textContent = latestUsefulLog(logs) || "İşlem hatasız sona erdi.";
  }
}

async function pollStatus() {
  try {
    const status = await api("/api/status");
    for (const field of ["started_at", "finished_at"]) {
      const value = status[field];
      if (value && new Date(value).getUTCFullYear() <= 1) status[field] = "";
    }
    $("#statusTitle").textContent = status.message || "Hazır";
    $("#statusAction").textContent = formatAction(status.action);
    $("#statusStarted").textContent = status.started_at ? new Date(status.started_at).toLocaleString("tr-TR") : "—";
    $("#statusFinished").textContent = status.finished_at ? new Date(status.finished_at).toLocaleString("tr-TR") : "—";
    $("#statusDuration").textContent = formatDuration(status.started_at, status.finished_at);
    $("#statusCase").textContent = status.case_dir || "—";
    $("#statusCase").title = status.case_dir || "";
    renderResult(status);
    const dot = $("#statusDot");
    dot.className = "status-dot";
    if (status.running) dot.classList.add("running");
    else if (status.finished_at) dot.classList.add(status.success ? "success" : "failed");
    $("#cancelButton").classList.toggle("hidden", !status.running);
    $$("button").forEach(button => {
      const monitorControl = button.id === "expandConsoleButton" || button.id === "scrollConsoleButton";
      if (button.id !== "cancelButton" && !button.classList.contains("tab") && !monitorControl) {
        button.disabled = status.running;
      }
    });
    if ((status.logs || []).length !== lastLogCount) {
      const consoleElement = $("#console");
      consoleElement.textContent = (status.logs || []).join("\n") || "Henüz kayıt yok.";
      consoleElement.scrollTop = consoleElement.scrollHeight;
      lastLogCount = (status.logs || []).length;
      $("#consoleLineCount").textContent = `${lastLogCount} satır`;
    }
    if (status.finished_at && status.finished_at !== lastFinishedAt) {
      if (lastFinishedAt || status.action) toast(status.message || "İşlem tamamlandı");
      lastFinishedAt = status.finished_at;
    }
  } catch (error) {
    $("#statusTitle").textContent = "Arayüz bağlantısı kesildi";
  }
}

async function initialise() {
  defaults = await api("/api/config");
  token = defaults.token;
  $("#quickCard").classList.toggle("hidden", !defaults.demo_available);
  $("#existingJob").value = defaults.demo_available ? defaults.demo_job : "";
  $("#verifyCaseDir").value = defaults.demo_available ? defaults.demo_case_dir : "";
  $("#verifyPublicKey").value = defaults.demo_available ? defaults.demo_public_key : (defaults.default_public_key || "");
  $("#agentLocal").value = defaults.default_agent;
  $('[name="output_directory"]').value = defaults.default_output || `${defaults.working_dir}/evidence`;
  $('[name="signing_key"]').value = defaults.default_signing_key || "";
  $('[name="tool_manifest"]').value = defaults.tool_manifest || "";
  $('[name="trust_public_key"]').value = defaults.trust_public_key || "";
  $('[name="known_hosts"]').value = defaults.default_known_hosts || "";
  if (defaults.default_private_key) {
    $('[name="private_key"]').placeholder = `Önerilen: ${defaults.default_private_key}`;
  }
  updateConditionalFields();
  await pollStatus();
  setInterval(pollStatus, 700);
}

async function browseForPath(button) {
  const target = $(button.dataset.browseTarget);
  if (!target) throw new Error("Yol alanı bulunamadı");
  const original = button.textContent;
  button.disabled = true;
  button.textContent = "Açılıyor…";
  try {
    const result = await api("/api/browse", {
      method: "POST",
      body: JSON.stringify({ kind: button.dataset.browseKind || "file" })
    });
    if (result.canceled) return;
    if (!result.path) throw new Error("Seçilen yol okunamadı");
    target.value = result.path;
    target.title = result.path;
    target.dispatchEvent(new Event("input", { bubbles: true }));
    target.dispatchEvent(new Event("change", { bubbles: true }));
    toast(button.dataset.browseKind === "directory" ? "Klasör seçildi." : "Dosya seçildi.");
  } finally {
    button.disabled = false;
    button.textContent = original;
  }
}

$$(".tab").forEach(button => button.addEventListener("click", () => switchTab(button.dataset.tab)));
$$('input[name="transport"], input[name="profile"]').forEach(input => input.addEventListener("change", updateConditionalFields));
$$(".browse-button").forEach(button => button.addEventListener("click", async () => {
  try {
    await browseForPath(button);
  } catch (error) {
    toast(error.message);
  }
}));
$("#diskSelect").addEventListener("change", event => {
  if (event.target.value === "") {
    clearSelectedDisk();
    return;
  }
  selectDisk(Number(event.target.value));
});
$("#localSourcePath").addEventListener("input", event => {
  $('[name="disk_path"]').value = event.target.value.trim();
});
for (const name of ["host", "port", "user", "password", "auth", "private_key", "known_hosts", "ca_file"]) {
  $(`[name="${name}"]`)?.addEventListener("input", () => {
    if (inventoryReady) clearInventory();
  });
}
$("#scanTargetButton").addEventListener("click", async () => {
  try {
    await scanTarget();
  } catch (error) {
    $("#targetScanStatus").textContent = error.message;
    toast(error.message);
  }
});
$("#trustHostKeyButton").addEventListener("click", async () => {
  if (!pendingHostKeyFingerprint) return;
  const target = connectionPayload();
  const button = $("#trustHostKeyButton");
  button.disabled = true;
  try {
    await api("/api/ssh-host-key", {
      method: "POST",
      body: JSON.stringify({
        job: target,
        trust_host_key: true,
        expected_fingerprint: pendingHostKeyFingerprint
      })
    });
    pendingHostKeyFingerprint = "";
    $("#hostKeyReview").classList.add("hidden");
    toast("SSH sunucu kimliği güven deposuna kaydedildi.");
    await scanTarget();
  } catch (error) {
    $("#targetScanStatus").textContent = error.message;
    toast(error.message);
  } finally {
    button.disabled = false;
  }
});
$("#cancelHostKeyButton").addEventListener("click", () => {
  pendingHostKeyFingerprint = "";
  $("#hostKeyReview").classList.add("hidden");
  $("#targetScanStatus").textContent = "SSH sunucu kimliği onaylanmadı; bağlantı kurulmadı.";
});

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

$("#scrollConsoleButton").addEventListener("click", () => {
  const consoleElement = $("#console");
  consoleElement.scrollTop = consoleElement.scrollHeight;
});

$("#expandConsoleButton").addEventListener("click", () => {
  const monitor = $(".monitor");
  const expanded = monitor.classList.toggle("console-expanded");
  $("#expandConsoleButton").textContent = expanded ? "Normal boyut" : "Büyüt";
  if (expanded) {
    const consoleElement = $("#console");
    consoleElement.scrollTop = consoleElement.scrollHeight;
  }
});

document.addEventListener("keydown", event => {
  if (event.key === "Escape" && $(".monitor").classList.contains("console-expanded")) {
    $(".monitor").classList.remove("console-expanded");
    $("#expandConsoleButton").textContent = "Büyüt";
  }
});

$("#shutdownButton").addEventListener("click", async () => {
  try {
    const result = await api("/api/shutdown", { method: "POST", body: "{}" });
    $("#statusTitle").textContent = "Kapatılıyor";
    toast(result.message);
    setTimeout(() => window.close(), 500);
  } catch (error) {
    toast(error.message);
  }
});

initialise().catch(error => toast(`Arayüz başlatılamadı: ${error.message}`));

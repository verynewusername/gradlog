(() => {
  const TOKEN_KEY = "gradlog_token";

  const state = {
    token: localStorage.getItem(TOKEN_KEY) || "",
    me: null,
    projects: [],
    experiments: [],
    runs: [],
    latestMetrics: [],
    artifacts: [],
    apiKeys: [],
    selectedProjectId: "",
    selectedExperimentId: "",
    selectedRunId: "",
  };

  const el = {
    authView: document.getElementById("authView"),
    appView: document.getElementById("appView"),
    oauthBtn: document.getElementById("oauthBtn"),
    tokenForm: document.getElementById("tokenForm"),
    tokenInput: document.getElementById("tokenInput"),
    authHint: document.getElementById("authHint"),
    logoutBtn: document.getElementById("logoutBtn"),
    statusBadge: document.getElementById("statusBadge"),
    toast: document.getElementById("toast"),

    projectForm: document.getElementById("projectForm"),
    projectName: document.getElementById("projectName"),
    projectDesc: document.getElementById("projectDesc"),
    projectList: document.getElementById("projectList"),

    experimentForm: document.getElementById("experimentForm"),
    experimentName: document.getElementById("experimentName"),
    experimentDesc: document.getElementById("experimentDesc"),
    experimentList: document.getElementById("experimentList"),

    runForm: document.getElementById("runForm"),
    runName: document.getElementById("runName"),
    runParams: document.getElementById("runParams"),
    runTags: document.getElementById("runTags"),
    runList: document.getElementById("runList"),
    runMeta: document.getElementById("runMeta"),
    runStatusForm: document.getElementById("runStatusForm"),
    runStatusSelect: document.getElementById("runStatusSelect"),

    metricForm: document.getElementById("metricForm"),
    metricKey: document.getElementById("metricKey"),
    metricValue: document.getElementById("metricValue"),
    metricStep: document.getElementById("metricStep"),
    metricTableBody: document.querySelector("#metricTable tbody"),

    artifactForm: document.getElementById("artifactForm"),
    artifactPath: document.getElementById("artifactPath"),
    artifactFile: document.getElementById("artifactFile"),
    artifactList: document.getElementById("artifactList"),

    apiKeyForm: document.getElementById("apiKeyForm"),
    apiKeyName: document.getElementById("apiKeyName"),
    apiKeyExpiry: document.getElementById("apiKeyExpiry"),
    apiKeyList: document.getElementById("apiKeyList"),
    newKeyBox: document.getElementById("newKeyBox"),
  };

  function toast(message, isError = false) {
    el.toast.textContent = message;
    el.toast.style.borderColor = isError ? "#ff5d73" : "#2b4254";
    el.toast.classList.remove("hidden");
    setTimeout(() => el.toast.classList.add("hidden"), 2600);
  }

  async function api(path, options = {}) {
    const headers = options.headers ? { ...options.headers } : {};
    if (!(options.body instanceof FormData)) {
      headers["Content-Type"] = headers["Content-Type"] || "application/json";
    }
    if (state.token) {
      headers.Authorization = `Bearer ${state.token}`;
    }

    const res = await fetch(path, { ...options, headers });
    const ct = res.headers.get("content-type") || "";
    const payload = ct.includes("application/json") ? await res.json() : await res.text();

    if (!res.ok) {
      const msg = payload && payload.error ? payload.error : `${res.status} ${res.statusText}`;
      throw new Error(msg);
    }
    return payload;
  }

  function setAuthUi(authed) {
    el.authView.classList.toggle("hidden", authed);
    el.appView.classList.toggle("hidden", !authed);
    el.logoutBtn.classList.toggle("hidden", !authed);
    el.statusBadge.textContent = authed && state.me ? `Signed in as ${state.me.email}` : "Disconnected";
  }

  function parseJSONOrEmpty(text, fieldName) {
    const raw = (text || "").trim();
    if (!raw) return {};
    try {
      const obj = JSON.parse(raw);
      if (obj && typeof obj === "object" && !Array.isArray(obj)) {
        return obj;
      }
      throw new Error("must be an object");
    } catch {
      throw new Error(`${fieldName} must be valid JSON object`);
    }
  }

  function timeFmt(value) {
    if (!value) return "-";
    return new Date(value).toLocaleString();
  }

  function renderSelectableList(target, items, selectedId, getLabel, onSelect, extraButtons = () => []) {
    target.innerHTML = "";
    if (!items.length) {
      const li = document.createElement("li");
      li.className = "hint";
      li.textContent = "No items yet.";
      target.appendChild(li);
      return;
    }

    items.forEach((item) => {
      const li = document.createElement("li");
      li.className = `item-row ${item.id === selectedId ? "active" : ""}`;

      const left = document.createElement("button");
      left.className = "btn btn-ghost";
      left.textContent = getLabel(item);
      left.onclick = () => onSelect(item.id);

      const actions = document.createElement("div");
      extraButtons(item).forEach((btn) => actions.appendChild(btn));

      li.appendChild(left);
      li.appendChild(actions);
      target.appendChild(li);
    });
  }

  async function loadMe() {
    state.me = await api("/api/v1/auth/me");
    setAuthUi(true);
  }

  async function loadProjects() {
    state.projects = await api("/api/v1/projects");
    renderProjects();

    if (!state.selectedProjectId && state.projects.length) {
      await selectProject(state.projects[0].id);
    }
  }

  function renderProjects() {
    renderSelectableList(
      el.projectList,
      state.projects,
      state.selectedProjectId,
      (p) => p.name,
      selectProject
    );
  }

  async function selectProject(id) {
    state.selectedProjectId = id;
    state.selectedExperimentId = "";
    state.selectedRunId = "";
    state.runs = [];
    state.latestMetrics = [];
    state.artifacts = [];
    renderProjects();
    await loadExperiments();
  }

  async function loadExperiments() {
    if (!state.selectedProjectId) {
      state.experiments = [];
      renderExperiments();
      return;
    }
    state.experiments = await api(`/api/v1/projects/${state.selectedProjectId}/experiments`);
    renderExperiments();

    if (!state.selectedExperimentId && state.experiments.length) {
      await selectExperiment(state.experiments[0].id);
    }
  }

  function renderExperiments() {
    renderSelectableList(
      el.experimentList,
      state.experiments,
      state.selectedExperimentId,
      (e) => e.name,
      selectExperiment
    );
  }

  async function selectExperiment(id) {
    state.selectedExperimentId = id;
    state.selectedRunId = "";
    state.latestMetrics = [];
    state.artifacts = [];
    renderExperiments();
    await loadRuns();
  }

  async function loadRuns() {
    if (!state.selectedExperimentId) {
      state.runs = [];
      renderRuns();
      return;
    }
    state.runs = await api(`/api/v1/experiments/${state.selectedExperimentId}/runs`);
    renderRuns();

    if (!state.selectedRunId && state.runs.length) {
      await selectRun(state.runs[0].id);
    } else if (!state.runs.length) {
      renderRunDetails(null);
    }
  }

  function runStatusButton(run, status) {
    const btn = document.createElement("button");
    btn.className = "btn btn-ghost";
    btn.textContent = status;
    btn.onclick = async () => {
      try {
        await api(`/api/v1/runs/${run.id}`, {
          method: "PATCH",
          body: JSON.stringify({ status }),
        });
        toast("Run updated");
        await loadRuns();
      } catch (e) {
        toast(e.message, true);
      }
    };
    return btn;
  }

  function renderRuns() {
    renderSelectableList(
      el.runList,
      state.runs,
      state.selectedRunId,
      (r) => `${r.name || "Unnamed run"} (${r.status})`,
      selectRun,
      (run) => [runStatusButton(run, "completed")]
    );
  }

  async function selectRun(id) {
    state.selectedRunId = id;
    renderRuns();

    const run = await api(`/api/v1/runs/${id}`);
    renderRunDetails(run);
    el.runStatusSelect.value = run.status;

    state.latestMetrics = await api(`/api/v1/runs/${id}/metrics/latest`);
    renderMetrics();

    state.artifacts = await api(`/api/v1/runs/${id}/artifacts`);
    renderArtifacts();
  }

  function renderRunDetails(run) {
    if (!run) {
      el.runMeta.textContent = "Select a run to view details.";
      el.metricTableBody.innerHTML = "";
      el.artifactList.innerHTML = "";
      return;
    }

    el.runMeta.textContent = [
      `Run: ${run.name || "Unnamed run"}`,
      `ID: ${run.id}`,
      `Status: ${run.status}`,
      `Start: ${timeFmt(run.start_time)}`,
      `End: ${timeFmt(run.end_time)}`,
      `Params: ${JSON.stringify(run.params || {}, null, 2)}`,
      `Tags: ${JSON.stringify(run.tags || {}, null, 2)}`,
    ].join("\n");
  }

  function renderMetrics() {
    el.metricTableBody.innerHTML = "";
    if (!state.latestMetrics.length) {
      const tr = document.createElement("tr");
      tr.innerHTML = "<td colspan='4'>No metrics yet.</td>";
      el.metricTableBody.appendChild(tr);
      return;
    }

    state.latestMetrics.forEach((m) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${m.key}</td><td>${m.value}</td><td>${m.step}</td><td>${timeFmt(m.timestamp)}</td>`;
      el.metricTableBody.appendChild(tr);
    });
  }

  function renderArtifacts() {
    el.artifactList.innerHTML = "";
    if (!state.artifacts.length) {
      const li = document.createElement("li");
      li.className = "hint";
      li.textContent = "No artifacts yet.";
      el.artifactList.appendChild(li);
      return;
    }

    state.artifacts.forEach((a) => {
      const li = document.createElement("li");
      li.className = "item-row";
      const label = document.createElement("span");
      label.textContent = `${a.path} (${a.file_size} B)`;

      const download = document.createElement("button");
      download.className = "btn btn-ghost";
      download.textContent = "Download";
      download.onclick = async () => {
        try {
          const res = await fetch(`/api/v1/artifacts/${a.id}/download`, {
            headers: { Authorization: `Bearer ${state.token}` },
          });
          if (!res.ok) throw new Error("Failed to download artifact");
          const blob = await res.blob();
          const url = URL.createObjectURL(blob);
          const anchor = document.createElement("a");
          anchor.href = url;
          anchor.download = a.file_name || "artifact";
          anchor.click();
          URL.revokeObjectURL(url);
        } catch (e) {
          toast(e.message, true);
        }
      };

      li.appendChild(label);
      li.appendChild(download);
      el.artifactList.appendChild(li);
    });
  }

  async function loadApiKeys() {
    state.apiKeys = await api("/api/v1/api-keys");
    el.apiKeyList.innerHTML = "";

    if (!state.apiKeys.length) {
      const li = document.createElement("li");
      li.className = "hint";
      li.textContent = "No keys yet.";
      el.apiKeyList.appendChild(li);
      return;
    }

    state.apiKeys.forEach((k) => {
      const li = document.createElement("li");
      li.className = "item-row";
      const left = document.createElement("span");
      left.textContent = `${k.name} (${k.key_prefix}...)`;

      const del = document.createElement("button");
      del.className = "btn btn-ghost";
      del.textContent = "Delete";
      del.onclick = async () => {
        try {
          await api(`/api/v1/api-keys/${k.id}`, { method: "DELETE" });
          toast("API key deleted");
          await loadApiKeys();
        } catch (e) {
          toast(e.message, true);
        }
      };

      li.appendChild(left);
      li.appendChild(del);
      el.apiKeyList.appendChild(li);
    });
  }

  async function bootstrapAuthed() {
    try {
      await loadMe();
      await loadProjects();
      await loadApiKeys();
      toast("Connected");
    } catch (e) {
      state.token = "";
      localStorage.removeItem(TOKEN_KEY);
      setAuthUi(false);
      el.authHint.textContent = e.message;
    }
  }

  function handleOAuthCallback() {
    if (!window.location.pathname.startsWith("/auth/callback")) {
      return;
    }
    const token = new URLSearchParams(window.location.search).get("token");
    if (!token) {
      el.authHint.textContent = "OAuth callback did not include a token.";
      setAuthUi(false);
      return;
    }
    state.token = token;
    localStorage.setItem(TOKEN_KEY, token);
    window.history.replaceState({}, "", "/");
  }

  function bindEvents() {
    el.oauthBtn.onclick = () => {
      window.location.href = "/api/v1/auth/google/login";
    };

    el.tokenForm.onsubmit = async (ev) => {
      ev.preventDefault();
      state.token = el.tokenInput.value.trim();
      if (!state.token) return;
      localStorage.setItem(TOKEN_KEY, state.token);
      await bootstrapAuthed();
    };

    el.logoutBtn.onclick = () => {
      state.token = "";
      state.me = null;
      localStorage.removeItem(TOKEN_KEY);
      setAuthUi(false);
      el.authHint.textContent = "Logged out.";
    };

    el.projectForm.onsubmit = async (ev) => {
      ev.preventDefault();
      try {
        await api("/api/v1/projects", {
          method: "POST",
          body: JSON.stringify({
            name: el.projectName.value.trim(),
            description: el.projectDesc.value.trim(),
          }),
        });
        el.projectForm.reset();
        await loadProjects();
        toast("Project created");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.experimentForm.onsubmit = async (ev) => {
      ev.preventDefault();
      if (!state.selectedProjectId) {
        toast("Select a project first", true);
        return;
      }
      try {
        await api(`/api/v1/projects/${state.selectedProjectId}/experiments`, {
          method: "POST",
          body: JSON.stringify({
            name: el.experimentName.value.trim(),
            description: el.experimentDesc.value.trim(),
          }),
        });
        el.experimentForm.reset();
        await loadExperiments();
        toast("Experiment created");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.runForm.onsubmit = async (ev) => {
      ev.preventDefault();
      if (!state.selectedExperimentId) {
        toast("Select an experiment first", true);
        return;
      }

      try {
        const params = parseJSONOrEmpty(el.runParams.value, "Params");
        const tags = parseJSONOrEmpty(el.runTags.value, "Tags");
        await api(`/api/v1/experiments/${state.selectedExperimentId}/runs`, {
          method: "POST",
          body: JSON.stringify({
            name: el.runName.value.trim() || null,
            params,
            tags,
          }),
        });
        el.runForm.reset();
        await loadRuns();
        toast("Run created");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.runStatusForm.onsubmit = async (ev) => {
      ev.preventDefault();
      if (!state.selectedRunId) {
        toast("Select a run first", true);
        return;
      }
      try {
        await api(`/api/v1/runs/${state.selectedRunId}`, {
          method: "PATCH",
          body: JSON.stringify({ status: el.runStatusSelect.value }),
        });
        await selectRun(state.selectedRunId);
        await loadRuns();
        toast("Run status updated");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.metricForm.onsubmit = async (ev) => {
      ev.preventDefault();
      if (!state.selectedRunId) {
        toast("Select a run first", true);
        return;
      }
      try {
        await api(`/api/v1/runs/${state.selectedRunId}/metrics`, {
          method: "POST",
          body: JSON.stringify({
            key: el.metricKey.value.trim(),
            value: Number(el.metricValue.value),
            step: Number(el.metricStep.value || "0"),
          }),
        });
        el.metricForm.reset();
        el.metricStep.value = "0";
        state.latestMetrics = await api(`/api/v1/runs/${state.selectedRunId}/metrics/latest`);
        renderMetrics();
        toast("Metric logged");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.artifactForm.onsubmit = async (ev) => {
      ev.preventDefault();
      if (!state.selectedRunId) {
        toast("Select a run first", true);
        return;
      }
      try {
        const fd = new FormData();
        fd.set("file", el.artifactFile.files[0]);
        if (el.artifactPath.value.trim()) {
          fd.set("path", el.artifactPath.value.trim());
        }
        await api(`/api/v1/runs/${state.selectedRunId}/artifacts/upload`, {
          method: "POST",
          body: fd,
        });
        el.artifactForm.reset();
        state.artifacts = await api(`/api/v1/runs/${state.selectedRunId}/artifacts`);
        renderArtifacts();
        toast("Artifact uploaded");
      } catch (e) {
        toast(e.message, true);
      }
    };

    el.apiKeyForm.onsubmit = async (ev) => {
      ev.preventDefault();
      try {
        const expiryRaw = el.apiKeyExpiry.value.trim();
        const payload = { name: el.apiKeyName.value.trim() };
        if (expiryRaw) {
          payload.expires_in = Number(expiryRaw);
        }
        const created = await api("/api/v1/api-keys", {
          method: "POST",
          body: JSON.stringify(payload),
        });
        el.apiKeyForm.reset();
        el.newKeyBox.textContent = `New key (shown once): ${created.key}`;
        await loadApiKeys();
        toast("API key created");
      } catch (e) {
        toast(e.message, true);
      }
    };
  }

  async function init() {
    handleOAuthCallback();
    bindEvents();
    if (state.token) {
      await bootstrapAuthed();
    } else {
      setAuthUi(false);
    }
  }

  init();
})();

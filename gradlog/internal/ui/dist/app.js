(() => {
  "use strict";

  const TOKEN_KEY = "gradlog_token";
  const THEME_KEY = "gradlog_theme";
  const ROUTE_KEY = "gradlog_route";

  /* ------------------------------------------------------------------ */
  /*  State                                                              */
  /* ------------------------------------------------------------------ */
  const state = {
    token: localStorage.getItem(TOKEN_KEY) || "",
    noauth: false,   // true when server is running with DEV_NOAUTH_EMAIL
    theme: "light",
    me: null,
    projects: [],
    experiments: [],
    runs: [],
    metricsGrouped: [],   // full metric history per key
    latestMetrics: [],
    artifacts: [],
    apiKeys: [],
    members: [],
    selectedProjectId: "",
    selectedExperimentId: "",
    selectedRunId: "",
    activeView: "run",       // run | members | apikeys
    activeSidebarTab: "projects",
    charts: {},              // key -> Chart instance
    refreshTimer: null,
  };

  /* ------------------------------------------------------------------ */
  /*  DOM refs                                                           */
  /* ------------------------------------------------------------------ */
  const $ = (id) => document.getElementById(id);
  const el = {
    authView: $("authView"),
    appView: $("appView"),
    oauthBtn: $("oauthBtn"),
    authHint: $("authHint"),
    themeToggle: $("themeToggle"),
    statusBadge: $("statusBadge"),
    toast: $("toast"),
    userMenuBtn: $("userMenuBtn"),
    userDropdown: $("userDropdown"),
    userAvatar: $("userAvatar"),
    userName: $("userName"),
    dropdownEmail: $("dropdownEmail"),
    logoutBtn: $("logoutBtn"),

    // Sidebar
    newProjectBtn: $("newProjectBtn"),
    projectFormWrap: $("projectFormWrap"),
    projectForm: $("projectForm"),
    projectName: $("projectName"),
    projectDesc: $("projectDesc"),
    cancelProjectBtn: $("cancelProjectBtn"),
    projectList: $("projectList"),

    newExperimentBtn: $("newExperimentBtn"),
    experimentFormWrap: $("experimentFormWrap"),
    experimentForm: $("experimentForm"),
    experimentName: $("experimentName"),
    experimentDesc: $("experimentDesc"),
    cancelExperimentBtn: $("cancelExperimentBtn"),
    experimentList: $("experimentList"),

    runList: $("runList"),

    // Content
    contentTitle: $("contentTitle"),
    contentSubtitle: $("contentSubtitle"),
    runActions: $("runActions"),
    runStatusForm: $("runStatusForm"),
    runStatusSelect: $("runStatusSelect"),
    runInfoGrid: $("runInfoGrid"),
    runInfoStatus: $("runInfoStatus"),
    runInfoStart: $("runInfoStart"),
    runInfoDuration: $("runInfoDuration"),
    runInfoId: $("runInfoId"),
    runParamsTags: $("runParamsTags"),
    runParamsDisplay: $("runParamsDisplay"),
    runTagsDisplay: $("runTagsDisplay"),

    chartsSection: $("chartsSection"),
    chartsContainer: $("chartsContainer"),
    refreshMetricsBtn: $("refreshMetricsBtn"),
    metricTableBody: document.querySelector("#metricTable tbody"),

    artifactsSection: $("artifactsSection"),
    artifactFileInput: $("artifactFileInput"),
    uploadProgress: $("uploadProgress"),
    uploadBar: $("uploadBar"),
    uploadText: $("uploadText"),
    downloadProgress: $("downloadProgress"),
    downloadBar: $("downloadBar"),
    downloadText: $("downloadText"),
    artifactList: $("artifactList"),

    // Members
    memberForm: $("memberForm"),
    memberEmail: $("memberEmail"),
    memberRole: $("memberRole"),
    memberList: $("memberList"),
    noProjectMembers: $("noProjectMembers"),
    membersContent: $("membersContent"),
    memberProjectScope: $("memberProjectScope"),

    // API Keys
    apiKeyForm: $("apiKeyForm"),
    apiKeyName: $("apiKeyName"),
    apiKeyExpiry: $("apiKeyExpiry"),
    newKeyBox: $("newKeyBox"),
    newKeyValue: $("newKeyValue"),
    copyKeyBtn: $("copyKeyBtn"),
    apiKeyList: $("apiKeyList"),
    apiKeyScope: $("apiKeyScope"),

    // Confirm dialog
    confirmOverlay: $("confirmOverlay"),
    confirmTitle: $("confirmTitle"),
    confirmMessage: $("confirmMessage"),
    confirmCancel: $("confirmCancel"),
    confirmOk: $("confirmOk"),
  };

  /* ------------------------------------------------------------------ */
  /*  Helpers                                                            */
  /* ------------------------------------------------------------------ */
  function toast(message, isError = false) {
    el.toast.textContent = message;
    el.toast.classList.toggle("toast-error", isError);
    el.toast.classList.remove("hidden");
    clearTimeout(toast._t);
    toast._t = setTimeout(() => el.toast.classList.add("hidden"), 2800);
  }

  function esc(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  function timeFmt(value) {
    if (!value) return "—";
    return new Date(value).toLocaleString();
  }

  function durationFmt(start, end) {
    if (!start) return "—";
    const s = new Date(start).getTime();
    const e = end ? new Date(end).getTime() : Date.now();
    const diff = Math.max(0, e - s);
    const secs = Math.floor(diff / 1000);
    if (secs < 60) return `${secs}s`;
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins}m ${secs % 60}s`;
    const hrs = Math.floor(mins / 60);
    return `${hrs}h ${mins % 60}m`;
  }

  function fileSizeFmt(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
  }

  function getSelectedProject() {
    return state.projects.find((project) => project.id === state.selectedProjectId) || null;
  }

  function updateSettingsScope() {
    const selectedProject = getSelectedProject();

    if (el.memberProjectScope) {
      if (selectedProject) {
        el.memberProjectScope.innerHTML = `<span class="scope-label">Project Scope</span><strong>${esc(selectedProject.name)}</strong><span class="scope-note">Members added here will only be added to this project.</span>`;
      } else {
        el.memberProjectScope.innerHTML = '<span class="scope-label">Project Scope</span><strong>No project selected</strong><span class="scope-note">Choose a project in the Explorer tab before adding members.</span>';
      }
    }

    if (el.apiKeyScope) {
      const selectedProjectText = selectedProject
        ? `Current project selection: ${esc(selectedProject.name)}.`
        : 'No project is currently selected.';
      el.apiKeyScope.innerHTML = `<span class="scope-label">Key Scope</span><strong>Account-wide</strong><span class="scope-note">${selectedProjectText} API keys are not project-specific in the current backend.</span>`;
    }
  }

  /* ------------------------------------------------------------------ */
  /*  Confirm dialog                                                     */
  /* ------------------------------------------------------------------ */
  let confirmResolve = null;
  function confirm(title, message, okLabel = "Delete") {
    el.confirmTitle.textContent = title;
    el.confirmMessage.textContent = message;
    el.confirmOk.textContent = okLabel;
    el.confirmOverlay.classList.remove("hidden");
    return new Promise((resolve) => { confirmResolve = resolve; });
  }
  function closeConfirm(result) {
    el.confirmOverlay.classList.add("hidden");
    if (confirmResolve) { confirmResolve(result); confirmResolve = null; }
  }

  /* ------------------------------------------------------------------ */
  /*  Theme                                                              */
  /* ------------------------------------------------------------------ */
  function setTheme(t) {
    state.theme = t === "dark" ? "dark" : "light";
    document.documentElement.setAttribute("data-theme", state.theme);
    localStorage.setItem(THEME_KEY, state.theme);
  }
  function initTheme() {
    const stored = localStorage.getItem(THEME_KEY);
    if (stored === "light" || stored === "dark") { setTheme(stored); return; }
    setTheme(window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  }

  /* ------------------------------------------------------------------ */
  /*  Routing (hash-based)                                               */
  /* ------------------------------------------------------------------ */
  function saveRoute() {
    const r = {
      p: state.selectedProjectId,
      e: state.selectedExperimentId,
      r: state.selectedRunId,
      v: state.activeView,
      t: state.activeSidebarTab,
    };
    const hash = btoa(JSON.stringify(r));
    if (window.location.hash !== "#" + hash) {
      history.replaceState(null, "", "#" + hash);
    }
  }

  function loadRoute() {
    const hash = window.location.hash.slice(1);
    if (!hash) return null;
    try { return JSON.parse(atob(hash)); } catch { return null; }
  }

  /* ------------------------------------------------------------------ */
  /*  API                                                                */
  /* ------------------------------------------------------------------ */
  async function api(path, options = {}) {
    const headers = options.headers ? { ...options.headers } : {};
    if (!(options.body instanceof FormData)) {
      headers["Content-Type"] = headers["Content-Type"] || "application/json";
    }
    // In noauth mode the server accepts requests with no Authorization header.
    if (state.token && !state.noauth) {
      headers.Authorization = `Bearer ${state.token}`;
    }
    const res = await fetch(path, { ...options, headers });
    if (res.status === 204) return null;
    const ct = res.headers.get("content-type") || "";
    const payload = ct.includes("application/json") ? await res.json() : await res.text();
    if (!res.ok) {
      const msg = payload?.error || `${res.status} ${res.statusText}`;
      throw new Error(msg);
    }
    return payload;
  }

  /* ------------------------------------------------------------------ */
  /*  Auth UI                                                            */
  /* ------------------------------------------------------------------ */
  function setAuthUi(authed) {
    el.authView.classList.toggle("hidden", authed);
    el.appView.classList.toggle("hidden", !authed);
    const online = authed && state.me;
    el.statusBadge.textContent = online ? "Online" : "Disconnected";
    el.statusBadge.classList.toggle("badge-online", !!online);
    el.statusBadge.classList.toggle("badge-offline", !online);
    if (online) {
      el.userName.textContent = state.me.name || state.me.email;
      el.dropdownEmail.textContent = state.me.email;
      el.userAvatar.src = state.me.picture_url || "";
      el.userAvatar.alt = state.me.name || "";
    }
  }

  /* ------------------------------------------------------------------ */
  /*  View switching                                                     */
  /* ------------------------------------------------------------------ */
  function showView(name) {
    state.activeView = name;
    document.querySelectorAll(".view").forEach((v) => v.classList.remove("active"));
    const viewMap = { run: "viewRun", members: "viewMembers", apikeys: "viewApiKeys" };
    const target = $(viewMap[name]);
    if (target) target.classList.add("active");

    // Update settings nav active state
    document.querySelectorAll(".settings-nav-btn").forEach((b) => {
      b.classList.toggle("active", b.dataset.settings === name);
    });

    saveRoute();
  }

  function switchSidebarTab(tab) {
    state.activeSidebarTab = tab;
    document.querySelectorAll(".sidebar-tab").forEach((t) => t.classList.toggle("active", t.dataset.tab === tab));
    document.querySelectorAll(".sidebar-tab-content").forEach((c) => c.classList.remove("active"));
    const target = $(tab === "projects" ? "tabProjects" : "tabSettings");
    if (target) target.classList.add("active");

    if (tab === "settings") {
      // Show the active settings view
      if (state.activeView !== "members" && state.activeView !== "apikeys") {
        showView("members");
      }
      // Update settings scope when switching to settings tab
      updateSettingsScope();
    } else {
      showView("run");
    }
    saveRoute();
  }

  /* ------------------------------------------------------------------ */
  /*  Projects                                                           */
  /* ------------------------------------------------------------------ */
  async function loadProjects() {
    state.projects = await api("/api/v1/projects");
    renderProjects();
    updateSettingsScope();
  }

  function renderProjects() {
    el.projectList.innerHTML = "";
    if (!state.projects.length) {
      el.projectList.innerHTML = '<li class="hint">No projects yet</li>';
      return;
    }
    state.projects.forEach((p) => {
      const li = document.createElement("li");
      li.className = `item-row ${p.id === state.selectedProjectId ? "active" : ""}`;

      const btn = document.createElement("button");
      btn.className = "item-label";
      btn.textContent = p.name;
      btn.onclick = () => selectProject(p.id);
      li.appendChild(btn);

      if (state.me && p.owner_id === state.me.id) {
        const delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "item-action-btn";
        delBtn.title = "Delete project";
        delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>';
        delBtn.onclick = (ev) => {
          ev.stopPropagation();
          deleteProject(p);
        };
        li.appendChild(delBtn);
      }

      el.projectList.appendChild(li);
    });
  }

  async function deleteProject(project) {
    const phrase = "I am sure";
    const typed = window.prompt(
      `This will permanently delete project "${project.name}" and all experiments, runs, metrics, and artifacts.\n\nType exactly \"${phrase}\" to continue.`
    );
    if (typed === null) return;
    if (typed.trim() !== phrase) {
      toast(`Deletion cancelled. Type exactly \"${phrase}\".`, true);
      return;
    }

    const ok = await confirm(
      "Delete Project Forever",
      `Final confirmation: delete \"${project.name}\" and all related data permanently?`,
      "Delete Forever"
    );
    if (!ok) return;

    try {
      await api(`/api/v1/projects/${project.id}`, { method: "DELETE" });

      if (state.selectedProjectId === project.id) {
        state.selectedProjectId = "";
        state.selectedExperimentId = "";
        state.selectedRunId = "";
        state.experiments = [];
        state.runs = [];
        state.latestMetrics = [];
        state.metricsGrouped = [];
        state.artifacts = [];
        destroyCharts();
        clearRunView();
      }

      await loadProjects();
      if (!state.selectedProjectId && state.projects.length) {
        await selectProject(state.projects[0].id);
      }
      toast("Project deleted permanently");
    } catch (e) {
      toast(e.message, true);
    }
  }

  async function selectProject(id) {
    state.selectedProjectId = id;
    state.selectedExperimentId = "";
    state.selectedRunId = "";
    state.experiments = [];
    state.runs = [];
    state.latestMetrics = [];
    state.metricsGrouped = [];
    state.artifacts = [];
    destroyCharts();
    renderProjects();
    renderExperiments();
    renderRuns();
    updateSettingsScope();
    clearRunView();
    await loadExperiments();
    if (state.activeView === "members") await loadMembers();
    saveRoute();
  }

  /* ------------------------------------------------------------------ */
  /*  Experiments                                                        */
  /* ------------------------------------------------------------------ */
  async function loadExperiments() {
    if (!state.selectedProjectId) {
      state.experiments = [];
      state.runs = [];
      renderExperiments();
      renderRuns();
      return;
    }
    state.experiments = await api(`/api/v1/projects/${state.selectedProjectId}/experiments`);
    renderExperiments();
    if (!state.experiments.length) {
      state.selectedExperimentId = "";
      state.selectedRunId = "";
      state.runs = [];
      renderRuns();
      clearRunView();
      return;
    }
    if (!state.selectedExperimentId && state.experiments.length) {
      await selectExperiment(state.experiments[0].id);
    }
  }

  function renderExperiments() {
    el.experimentList.innerHTML = "";
    if (!state.experiments.length) {
      el.experimentList.innerHTML = '<li class="hint">No experiments yet</li>';
      return;
    }
    state.experiments.forEach((e) => {
      const li = document.createElement("li");
      li.className = `item-row ${e.id === state.selectedExperimentId ? "active" : ""}`;
      const btn = document.createElement("button");
      btn.className = "item-label";
      btn.textContent = e.name;
      btn.onclick = () => selectExperiment(e.id);
      li.appendChild(btn);

      const delBtn = document.createElement("button");
      delBtn.type = "button";
      delBtn.className = "item-action-btn";
      delBtn.title = "Delete experiment";
      delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>';
      delBtn.onclick = (ev) => {
        ev.stopPropagation();
        deleteExperiment(e);
      };
      li.appendChild(delBtn);

      el.experimentList.appendChild(li);
    });
  }

  async function deleteExperiment(exp) {
    const phrase = "I am sure";
    const typed = window.prompt(
      `This will permanently delete experiment "${exp.name}" and all of its runs, metrics, and artifacts.\n\nType exactly \"${phrase}\" to continue.`
    );
    if (typed === null) return;
    if (typed.trim() !== phrase) {
      toast(`Deletion cancelled. Type exactly \"${phrase}\".`, true);
      return;
    }

    const ok = await confirm(
      "Delete Experiment Forever",
      `Final confirmation: delete \"${exp.name}\" and all its data permanently?`,
      "Delete Forever"
    );
    if (!ok) return;

    try {
      await api(`/api/v1/experiments/${exp.id}`, { method: "DELETE" });

      if (state.selectedExperimentId === exp.id) {
        state.selectedExperimentId = "";
        state.selectedRunId = "";
        state.runs = [];
        state.latestMetrics = [];
        state.metricsGrouped = [];
        state.artifacts = [];
        destroyCharts();
        clearRunView();
      }

      await loadExperiments();
      toast("Experiment deleted permanently");
    } catch (e) {
      toast(e.message, true);
    }
  }

  async function selectExperiment(id) {
    state.selectedExperimentId = id;
    state.selectedRunId = "";
    state.latestMetrics = [];
    state.metricsGrouped = [];
    state.artifacts = [];
    destroyCharts();
    renderExperiments();
    clearRunView();
    await loadRuns();
    saveRoute();
  }

  /* ------------------------------------------------------------------ */
  /*  Runs                                                               */
  /* ------------------------------------------------------------------ */
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
      clearRunView();
    }
  }

  function renderRuns() {
    el.runList.innerHTML = "";
    if (!state.runs.length) {
      el.runList.innerHTML = '<li class="hint">No runs yet</li>';
      return;
    }
    state.runs.forEach((run) => {
      const li = document.createElement("li");
      li.className = `item-row ${run.id === state.selectedRunId ? "active" : ""}`;
      const btn = document.createElement("button");
      btn.className = "item-label";
      btn.innerHTML = `${esc(run.name || "Unnamed run")} <span class="status-pill status-${esc(run.status)}">${esc(run.status)}</span>`;
      btn.onclick = () => selectRun(run.id);
      li.appendChild(btn);

      const delBtn = document.createElement("button");
      delBtn.type = "button";
      delBtn.className = "item-action-btn";
      delBtn.title = "Delete run";
      delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>';
      delBtn.onclick = (ev) => {
        ev.stopPropagation();
        deleteRun(run);
      };
      li.appendChild(delBtn);

      el.runList.appendChild(li);
    });
  }

  async function deleteRun(run) {
    const phrase = "I am sure";
    const typed = window.prompt(
      `This will permanently delete run \"${run.name || run.id}\" and all related metrics and artifacts.\n\nType exactly \"${phrase}\" to continue.`
    );
    if (typed === null) return;
    if (typed.trim() !== phrase) {
      toast(`Deletion cancelled. Type exactly \"${phrase}\".`, true);
      return;
    }

    const ok = await confirm(
      "Delete Run Forever",
      `Final confirmation: permanently delete \"${run.name || run.id}\"?`,
      "Delete Forever"
    );
    if (!ok) return;

    try {
      await api(`/api/v1/runs/${run.id}`, { method: "DELETE" });

      if (state.selectedRunId === run.id) {
        state.selectedRunId = "";
        state.latestMetrics = [];
        state.metricsGrouped = [];
        state.artifacts = [];
        destroyCharts();
        clearRunView();
      }

      await loadRuns();
      toast("Run deleted permanently");
    } catch (e) {
      toast(e.message, true);
    }
  }

  function clearRunView() {
    el.contentTitle.textContent = "Select a run";
    el.contentSubtitle.textContent = "Choose a project, experiment, and run from the sidebar";
    el.runActions.classList.add("hidden");
    el.runInfoGrid.classList.add("hidden");
    el.runParamsTags.classList.add("hidden");
    el.chartsSection.classList.add("hidden");
    el.artifactsSection.classList.add("hidden");
    destroyCharts();
  }

  async function selectRun(id) {
    state.selectedRunId = id;
    renderRuns();

    const run = await api(`/api/v1/runs/${id}`);
    renderRunDetails(run);

    // Load metrics (full history for charts)
    state.metricsGrouped = await api(`/api/v1/runs/${id}/metrics`);
    state.latestMetrics = await api(`/api/v1/runs/${id}/metrics/latest`);
    renderMetrics();
    renderCharts();

    // Load artifacts
    state.artifacts = await api(`/api/v1/runs/${id}/artifacts`);
    renderArtifacts();

    saveRoute();
  }

  function renderRunDetails(run) {
    if (!run) { clearRunView(); return; }

    const proj = state.projects.find((p) => p.id === state.selectedProjectId);
    const exp = state.experiments.find((e) => e.id === state.selectedExperimentId);
    el.contentTitle.textContent = run.name || "Unnamed run";
    el.contentSubtitle.textContent = `${proj?.name || ""} / ${exp?.name || ""}`;

    el.runActions.classList.remove("hidden");
    el.runStatusSelect.value = run.status;

    el.runInfoGrid.classList.remove("hidden");
    el.runInfoStatus.innerHTML = `<span class="status-pill status-${esc(run.status)}">${esc(run.status)}</span>`;
    el.runInfoStart.textContent = timeFmt(run.start_time);
    el.runInfoDuration.textContent = durationFmt(run.start_time, run.end_time);
    el.runInfoId.textContent = run.id;

    // Params & Tags
    const params = run.params || {};
    const tags = run.tags || {};
    const hasParams = Object.keys(params).length > 0;
    const hasTags = Object.keys(tags).length > 0;
    el.runParamsTags.classList.toggle("hidden", !hasParams && !hasTags);
    renderKV(el.runParamsDisplay, params);
    renderKV(el.runTagsDisplay, tags);

    el.chartsSection.classList.remove("hidden");
    el.artifactsSection.classList.remove("hidden");
  }

  function renderKV(target, obj) {
    target.innerHTML = "";
    const keys = Object.keys(obj);
    if (!keys.length) {
      target.innerHTML = '<span class="hint">None</span>';
      return;
    }
    keys.forEach((k) => {
      const keyEl = document.createElement("span");
      keyEl.className = "kv-key";
      keyEl.textContent = k;
      const valEl = document.createElement("span");
      valEl.className = "kv-val";
      valEl.textContent = typeof obj[k] === "object" ? JSON.stringify(obj[k]) : String(obj[k]);
      target.appendChild(keyEl);
      target.appendChild(valEl);
    });
  }

  /* ------------------------------------------------------------------ */
  /*  Metrics & Charts                                                   */
  /* ------------------------------------------------------------------ */
  const CHART_COLORS = [
    "var(--chart-line-1)", "var(--chart-line-2)", "var(--chart-line-3)",
    "var(--chart-line-4)", "var(--chart-line-5)", "var(--chart-line-6)",
  ];

  // Resolved CSS variable colors for Chart.js
  function resolveColor(cssVar) {
    const temp = document.createElement("div");
    temp.style.color = cssVar;
    document.body.appendChild(temp);
    const color = getComputedStyle(temp).color;
    document.body.removeChild(temp);
    return color;
  }

  // Determine if a metric key should be charted (like MLflow: chart all numeric series)
  function isChartableKey(key) {
    return true; // Chart all metric keys — they're all numeric time series
  }

  function destroyCharts() {
    Object.values(state.charts).forEach((c) => c.destroy());
    state.charts = {};
    if (el.chartsContainer) el.chartsContainer.innerHTML = "";
  }

  function renderCharts() {
    destroyCharts();
    if (!state.metricsGrouped.length) {
      el.chartsContainer.innerHTML = '<div class="hint" style="padding:16px">No metrics logged yet.</div>';
      return;
    }

    const textColor = resolveColor("var(--text-3)");
    const gridColor = resolveColor("var(--border)");

    state.metricsGrouped.forEach((group, idx) => {
      if (!isChartableKey(group.key) || !group.history?.length) return;

      const card = document.createElement("div");
      card.className = "chart-card";
      const title = document.createElement("div");
      title.className = "chart-card-title";
      title.textContent = group.key;
      card.appendChild(title);

      const wrap = document.createElement("div");
      wrap.className = "chart-canvas-wrap";
      const canvas = document.createElement("canvas");
      wrap.appendChild(canvas);
      card.appendChild(wrap);
      el.chartsContainer.appendChild(card);

      const lineColor = resolveColor(CHART_COLORS[idx % CHART_COLORS.length]);

      const chart = new Chart(canvas, {
        type: "line",
        data: {
          labels: group.history.map((m) => m.step),
          datasets: [{
            label: group.key,
            data: group.history.map((m) => m.value),
            borderColor: lineColor,
            backgroundColor: lineColor + "20",
            borderWidth: 2,
            pointRadius: group.history.length > 50 ? 0 : 3,
            pointHoverRadius: 5,
            tension: 0.3,
            fill: true,
          }],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          interaction: { mode: "index", intersect: false },
          plugins: {
            legend: { display: false },
            tooltip: {
              backgroundColor: resolveColor("var(--surface)"),
              titleColor: resolveColor("var(--text)"),
              bodyColor: resolveColor("var(--text-2)"),
              borderColor: resolveColor("var(--border)"),
              borderWidth: 1,
              padding: 10,
              cornerRadius: 8,
              displayColors: false,
              callbacks: {
                title: (items) => `Step ${items[0].label}`,
                label: (item) => `${group.key}: ${item.parsed.y.toPrecision(6)}`,
              },
            },
          },
          scales: {
            x: {
              title: { display: true, text: "Step", color: textColor, font: { size: 11 } },
              ticks: { color: textColor, font: { size: 10 }, maxTicksLimit: 10 },
              grid: { color: gridColor, drawBorder: false },
            },
            y: {
              title: { display: true, text: group.key, color: textColor, font: { size: 11 } },
              ticks: { color: textColor, font: { size: 10 }, maxTicksLimit: 8 },
              grid: { color: gridColor, drawBorder: false },
            },
          },
        },
      });
      state.charts[group.key] = chart;
    });
  }

  function renderMetrics() {
    el.metricTableBody.innerHTML = "";
    if (!state.latestMetrics.length) {
      const tr = document.createElement("tr");
      tr.innerHTML = '<td colspan="4" class="hint">No metrics yet</td>';
      el.metricTableBody.appendChild(tr);
      return;
    }
    state.latestMetrics.forEach((m) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `<td><strong>${esc(m.key)}</strong></td><td>${m.value.toPrecision(6)}</td><td>${m.step}</td><td>${timeFmt(m.timestamp)}</td>`;
      el.metricTableBody.appendChild(tr);
    });
  }

  /* ------------------------------------------------------------------ */
  /*  Artifacts                                                          */
  /* ------------------------------------------------------------------ */
  function renderArtifacts() {
    el.artifactList.innerHTML = "";
    if (!state.artifacts.length) {
      el.artifactList.innerHTML = '<div class="hint" style="padding:8px">No artifacts yet.</div>';
      return;
    }
    state.artifacts.forEach((a) => {
      const row = document.createElement("div");
      row.className = "artifact-row";

      const icon = document.createElement("div");
      icon.className = "artifact-icon";
      icon.innerHTML = '<svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4z" clip-rule="evenodd"/></svg>';

      const info = document.createElement("div");
      info.className = "artifact-info";
      info.innerHTML = `<div class="artifact-name">${esc(a.path || a.file_name)}</div><div class="artifact-meta">${fileSizeFmt(a.file_size)} · ${timeFmt(a.created_at)}</div>`;

      const actions = document.createElement("div");
      actions.className = "artifact-actions";

      const dlBtn = document.createElement("button");
      dlBtn.type = "button";
      dlBtn.className = "btn btn-ghost btn-sm";
      dlBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clip-rule="evenodd"/></svg> Download';
      dlBtn.onclick = (ev) => {
        ev.preventDefault();
        ev.stopPropagation();
        downloadArtifact(a, dlBtn);
      };

      const delBtn = document.createElement("button");
      delBtn.className = "btn btn-ghost btn-sm";
      delBtn.style.color = "var(--red)";
      delBtn.style.borderColor = "var(--red-bg)";
      delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>';
      delBtn.title = "Delete artifact";
      delBtn.onclick = () => deleteArtifact(a);

      actions.appendChild(dlBtn);
      actions.appendChild(delBtn);
      row.appendChild(icon);
      row.appendChild(info);
      row.appendChild(actions);
      el.artifactList.appendChild(row);
    });
  }

  async function downloadArtifact(a, btn) {
    if (btn) {
      btn.disabled = true;
      btn.classList.add("is-loading");
      btn.innerHTML = '<svg class="spin" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9" opacity="0.25"></circle><path d="M21 12a9 9 0 00-9-9"></path></svg> Downloading...';
    }
    
    // Show download progress bar
    el.downloadProgress.classList.remove("hidden");
    el.downloadBar.style.width = "0%";
    el.downloadText.textContent = "Starting download...";

    try {
      toast(`Starting download: ${a.file_name || a.path || "artifact"}`);
      const dlHeaders = {};
      if (state.token && !state.noauth) dlHeaders.Authorization = `Bearer ${state.token}`;
      
      // Get file size from headers if available
      const headRes = await fetch(`/api/v1/artifacts/${a.id}/download`, { 
        method: "HEAD", 
        headers: dlHeaders, 
        cache: "no-store" 
      });
      const contentLength = headRes.headers.get("content-length");
      const fileSize = contentLength ? parseInt(contentLength, 10) : a.file_size || 0;
      
      const res = await fetch(`/api/v1/artifacts/${a.id}/download`, { headers: dlHeaders, cache: "no-store" });
      if (!res.ok) {
        let msg = "Download failed";
        try {
          const err = await res.json();
          if (err && err.error) msg = err.error;
        } catch {
          // ignore parse failures and keep generic message
        }

        throw new Error(msg);
      }

      // Prefer server-provided filename when available.
      let filename = a.file_name || "artifact";
      const cd = res.headers.get("content-disposition") || "";
      const m = cd.match(/filename\*=UTF-8''([^;]+)|filename="?([^";]+)"?/i);
      if (m) {
        filename = decodeURIComponent((m[1] || m[2] || "").trim()) || filename;
      }

      // Check file size and warn user if it's very large
      const GB = 1024 * 1024 * 1024;
      if (fileSize > 2 * GB) {
        toast(`Warning: Large file (${fileSizeFmt(fileSize)}). Download may take a while.`, true);
      }

      // Stream straight to disk when the browser supports File System Access API.
      if (typeof window.showSaveFilePicker === "function" && res.body && typeof res.body.pipeTo === "function") {
        try {
          const handle = await window.showSaveFilePicker({ 
            suggestedName: filename,
            types: [{
              description: 'All Files',
              accept: { '*/*': ['.*'] }
            }]
          });
          const writable = await handle.createWritable();
          
          // Show progress during streaming
          if (fileSize > 0) {
            let downloaded = 0;
            const reader = res.body.getReader();
            const stream = new ReadableStream({
              start(controller) {
                function push() {
                  reader.read().then(({ done, value }) => {
                    if (done) {
                      controller.close();
                      return;
                    }
                    downloaded += value.length;
                    const progress = Math.round((downloaded / fileSize) * 100);
                    el.downloadBar.style.width = progress + "%";
                    el.downloadText.textContent = `Downloading ${filename}... ${progress}%`;
                    controller.enqueue(value);
                    push();
                  }).catch(controller.error.bind(controller));
                }
                push();
              }
            });
            await stream.pipeTo(writable);
          } else {
            await res.body.pipeTo(writable);
          }
        } catch (e) {
          // If File System Access API fails, fall back to blob method
          console.warn("File System Access API failed, falling back to blob method:", e);
          await downloadAsBlob(res, filename, fileSize);
        }
      } else {
        // Fallback path for browsers without direct disk streaming support.
        await downloadAsBlob(res, filename, fileSize);
      }
      
      el.downloadText.textContent = "Download complete";
      el.downloadBar.style.width = "100%";
      setTimeout(() => el.downloadProgress.classList.add("hidden"), 1500);
    } catch (e) {
      toast(e.message, true);
      el.downloadProgress.classList.add("hidden");
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.classList.remove("is-loading");
        btn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z" clip-rule="evenodd"/></svg> Download';
      }
    }
  }

  async function downloadAsBlob(res, filename, fileSize) {
    // For blob method, we can't show progress easily, so just show a message
    el.downloadText.textContent = `Downloading ${filename}... (large file, please wait)`;
    
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = filename;
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    // Revoke asynchronously so Safari has time to start the download.
    setTimeout(() => URL.revokeObjectURL(url), 1500);
  }

  async function downloadAsBlob(res, filename) {
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = filename;
    anchor.style.display = "none";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    // Revoke asynchronously so Safari has time to start the download.
    setTimeout(() => URL.revokeObjectURL(url), 1500);
  }

  async function deleteArtifact(a) {
    const ok = await confirm(
      "Delete Artifact",
      `Are you sure you want to permanently delete "${a.path || a.file_name}"? This action cannot be undone.`,
      "Delete"
    );
    if (!ok) return;
    try {
      await api(`/api/v1/artifacts/${a.id}`, { method: "DELETE" });
      toast("Artifact deleted");
      state.artifacts = await api(`/api/v1/runs/${state.selectedRunId}/artifacts`);
      renderArtifacts();
    } catch (e) {
      toast(e.message, true);
    }
  }

  async function uploadArtifact(file) {
    if (!state.selectedRunId) { toast("Select a run first", true); return; }
    el.uploadProgress.classList.remove("hidden");
    el.uploadBar.style.width = "0%";
    el.uploadText.textContent = `Uploading ${file.name}...`;

    try {
      const fd = new FormData();
      fd.set("file", file);
      fd.set("path", file.name);

      const xhr = new XMLHttpRequest();
      xhr.open("POST", `/api/v1/runs/${state.selectedRunId}/artifacts/upload`);
      if (state.token && !state.noauth) {
        xhr.setRequestHeader("Authorization", `Bearer ${state.token}`);
      }

      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          const pct = Math.round((e.loaded / e.total) * 100);
          el.uploadBar.style.width = pct + "%";
          el.uploadText.textContent = `Uploading ${file.name}... ${pct}%`;
        }
      };

      await new Promise((resolve, reject) => {
        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) resolve();
          else reject(new Error(xhr.responseText || "Upload failed"));
        };
        xhr.onerror = () => reject(new Error("Upload failed"));
        xhr.send(fd);
      });

      toast("Artifact uploaded");
      state.artifacts = await api(`/api/v1/runs/${state.selectedRunId}/artifacts`);
      renderArtifacts();
    } catch (e) {
      toast(e.message, true);
    } finally {
      el.uploadProgress.classList.add("hidden");
      el.artifactFileInput.value = "";
    }
  }

  /* ------------------------------------------------------------------ */
  /*  Members                                                            */
  /* ------------------------------------------------------------------ */
  async function loadMembers() {
    updateSettingsScope();
    if (!state.selectedProjectId) {
      state.members = [];
      el.noProjectMembers?.classList.remove("hidden");
      el.membersContent?.classList.add("hidden");
      renderMembers();
      return;
    }
    el.noProjectMembers?.classList.add("hidden");
    el.membersContent?.classList.remove("hidden");
    try {
      state.members = await api(`/api/v1/projects/${state.selectedProjectId}/members`);
    } catch {
      state.members = [];
    }
    renderMembers();
  }

  function renderMembers() {
    el.memberList.innerHTML = "";
    if (!state.members.length) {
      el.memberList.innerHTML = '<div class="hint" style="padding:8px">No members.</div>';
      return;
    }
    state.members.forEach((m) => {
      const row = document.createElement("div");
      row.className = "member-row";

      const avatar = document.createElement("img");
      avatar.className = "member-avatar";
      avatar.src = m.picture_url || "";
      avatar.alt = m.name || m.email;

      const info = document.createElement("div");
      info.className = "member-info";
      info.innerHTML = `<div class="member-name">${esc(m.name || m.email)}</div><div class="member-email">${esc(m.email)}</div>`;

      const role = document.createElement("span");
      role.className = `member-role role-${m.role}`;
      role.textContent = m.role;

      row.appendChild(avatar);
      row.appendChild(info);
      row.appendChild(role);

      // Remove button (not for owner)
      if (m.role !== "owner") {
        const delBtn = document.createElement("button");
        delBtn.className = "btn btn-ghost btn-sm";
        delBtn.style.color = "var(--red)";
        delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"/></svg>';
        delBtn.title = "Remove member";
        delBtn.onclick = async () => {
          const ok = await confirm("Remove Member", `Remove ${m.name || m.email} from this project?`, "Remove");
          if (!ok) return;
          try {
            await api(`/api/v1/projects/${state.selectedProjectId}/members/${m.user_id}`, { method: "DELETE" });
            toast("Member removed");
            await loadMembers();
          } catch (e) {
            toast(e.message, true);
          }
        };
        row.appendChild(delBtn);
      }

      el.memberList.appendChild(row);
    });
  }

  /* ------------------------------------------------------------------ */
  /*  API Keys                                                           */
  /* ------------------------------------------------------------------ */
  async function loadApiKeys() {
    updateSettingsScope();
    try {
      state.apiKeys = await api("/api/v1/api-keys");
    } catch {
      state.apiKeys = [];
    }
    renderApiKeys();
  }

  function renderApiKeys() {
    el.apiKeyList.innerHTML = "";
    if (!state.apiKeys.length) {
      el.apiKeyList.innerHTML = '<div class="hint" style="padding:8px">No API keys yet.</div>';
      return;
    }
    state.apiKeys.forEach((k) => {
      const row = document.createElement("div");
      row.className = "apikey-row";

      const icon = document.createElement("div");
      icon.className = "apikey-icon";
      icon.innerHTML = '<svg width="16" height="16" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 8a6 6 0 01-7.743 5.743L10 14l-1 1-1 1H6v2H2v-4l4.257-4.257A6 6 0 1118 8zm-6-4a1 1 0 100 2 2 2 0 012 2 1 1 0 102 0 4 4 0 00-4-4z" clip-rule="evenodd"/></svg>';

      const info = document.createElement("div");
      info.className = "apikey-info";
      const expiry = k.expires_at ? `Expires ${timeFmt(k.expires_at)}` : "No expiry";
      const lastUsed = k.last_used_at ? `Last used ${timeFmt(k.last_used_at)}` : "Never used";
      info.innerHTML = `<div class="apikey-name">${esc(k.name)}</div><div class="apikey-meta">${esc(k.key_prefix)}... · ${expiry} · ${lastUsed}</div>`;

      const delBtn = document.createElement("button");
      delBtn.className = "btn btn-ghost btn-sm";
      delBtn.style.color = "var(--red)";
      delBtn.innerHTML = '<svg width="12" height="12" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clip-rule="evenodd"/></svg>';
      delBtn.title = "Delete key";
      delBtn.onclick = async () => {
        const ok = await confirm("Delete API Key", `Delete key "${k.name}"? Any scripts using this key will stop working.`, "Delete");
        if (!ok) return;
        try {
          await api(`/api/v1/api-keys/${k.id}`, { method: "DELETE" });
          toast("API key deleted");
          await loadApiKeys();
        } catch (e) {
          toast(e.message, true);
        }
      };

      row.appendChild(icon);
      row.appendChild(info);
      row.appendChild(delBtn);
      el.apiKeyList.appendChild(row);
    });
  }

  /* ------------------------------------------------------------------ */
  /*  Bootstrap                                                          */
  /* ------------------------------------------------------------------ */
  async function loadMe() {
    state.me = await api("/api/v1/auth/me");
    setAuthUi(true);
  }

  async function bootstrapAuthed() {
    try {
      await loadMe();
      await loadProjects();
      await loadApiKeys();

      // Restore route
      const route = loadRoute();
      if (route) {
        if (route.t) switchSidebarTab(route.t);
        if (route.p && state.projects.find((p) => p.id === route.p)) {
          await selectProject(route.p);
          if (route.e && state.experiments.find((e) => e.id === route.e)) {
            await selectExperiment(route.e);
            if (route.r && state.runs.find((r) => r.id === route.r)) {
              await selectRun(route.r);
            }
          }
        }
        if (route.v) showView(route.v);
        if (route.v === "members") await loadMembers();
      } else if (!state.selectedProjectId && state.projects.length) {
        await selectProject(state.projects[0].id);
      }

      toast("Connected");

      // Auto-refresh metrics every 10s for running runs
      clearInterval(state.refreshTimer);
      state.refreshTimer = setInterval(async () => {
        if (!state.selectedRunId) return;
        const run = state.runs.find((r) => r.id === state.selectedRunId);
        if (run?.status !== "running") return;
        try {
          state.metricsGrouped = await api(`/api/v1/runs/${state.selectedRunId}/metrics`);
          state.latestMetrics = await api(`/api/v1/runs/${state.selectedRunId}/metrics/latest`);
          renderMetrics();
          renderCharts();
          // Also refresh artifacts
          state.artifacts = await api(`/api/v1/runs/${state.selectedRunId}/artifacts`);
          renderArtifacts();
        } catch { /* ignore refresh errors */ }
      }, 10000);

    } catch (e) {
      state.token = "";
      localStorage.removeItem(TOKEN_KEY);
      setAuthUi(false);
      el.authHint.textContent = e.message;
    }
  }

  function handleOAuthCallback() {
    if (!window.location.pathname.startsWith("/auth/callback")) return;
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

  /* ------------------------------------------------------------------ */
  /*  Event bindings                                                     */
  /* ------------------------------------------------------------------ */
  function bindEvents() {
    const onClick = (node, handler) => { if (node) node.onclick = handler; };
    const onSubmit = (node, handler) => { if (node) node.onsubmit = handler; };
    const onChange = (node, handler) => { if (node) node.onchange = handler; };

    // Theme
    onClick(el.themeToggle, () => setTheme(state.theme === "dark" ? "light" : "dark"));

    // OAuth
    onClick(el.oauthBtn, () => { window.location.href = "/api/v1/auth/google/login"; });

    // User menu
    onClick(el.userMenuBtn, (e) => {
      e.stopPropagation();
      el.userDropdown.classList.toggle("hidden");
    });
    document.addEventListener("click", () => el.userDropdown.classList.add("hidden"));

    // Logout
    onClick(el.logoutBtn, () => {
      state.token = "";
      state.me = null;
      localStorage.removeItem(TOKEN_KEY);
      clearInterval(state.refreshTimer);
      setAuthUi(false);
      el.authHint.textContent = "Signed out.";
      window.history.replaceState({}, "", "/");
    });

    // Sidebar tabs
    document.querySelectorAll(".sidebar-tab").forEach((tab) => {
      tab.onclick = () => switchSidebarTab(tab.dataset.tab);
    });

    // Settings nav
    document.querySelectorAll(".settings-nav-btn").forEach((btn) => {
      btn.onclick = () => {
        showView(btn.dataset.settings);
        if (btn.dataset.settings === "members") loadMembers();
        if (btn.dataset.settings === "apikeys") loadApiKeys();
      };
    });

    // Confirm dialog
    onClick(el.confirmCancel, () => closeConfirm(false));
    onClick(el.confirmOk, () => closeConfirm(true));
    onClick(el.confirmOverlay, (e) => { if (e.target === el.confirmOverlay) closeConfirm(false); });

    // Project form
    onClick(el.newProjectBtn, () => el.projectFormWrap.classList.toggle("hidden"));
    onClick(el.cancelProjectBtn, () => el.projectFormWrap.classList.add("hidden"));
    onSubmit(el.projectForm, async (ev) => {
      ev.preventDefault();
      try {
        await api("/api/v1/projects", {
          method: "POST",
          body: JSON.stringify({ name: el.projectName.value.trim(), description: el.projectDesc.value.trim() }),
        });
        el.projectForm.reset();
        el.projectFormWrap.classList.add("hidden");
        await loadProjects();
        toast("Project created");
      } catch (e) { toast(e.message, true); }
    });

    // Experiment form
    onClick(el.newExperimentBtn, () => el.experimentFormWrap.classList.toggle("hidden"));
    onClick(el.cancelExperimentBtn, () => el.experimentFormWrap.classList.add("hidden"));
    onSubmit(el.experimentForm, async (ev) => {
      ev.preventDefault();
      if (!state.selectedProjectId) { toast("Select a project first", true); return; }
      try {
        await api(`/api/v1/projects/${state.selectedProjectId}/experiments`, {
          method: "POST",
          body: JSON.stringify({ name: el.experimentName.value.trim(), description: el.experimentDesc.value.trim() }),
        });
        el.experimentForm.reset();
        el.experimentFormWrap.classList.add("hidden");
        await loadExperiments();
        toast("Experiment created");
      } catch (e) { toast(e.message, true); }
    });

    // Run status update
    onSubmit(el.runStatusForm, async (ev) => {
      ev.preventDefault();
      if (!state.selectedRunId) return;
      try {
        await api(`/api/v1/runs/${state.selectedRunId}`, {
          method: "PATCH",
          body: JSON.stringify({ status: el.runStatusSelect.value }),
        });
        await loadRuns();
        await selectRun(state.selectedRunId);
        toast("Status updated");
      } catch (e) { toast(e.message, true); }
    });

    // Refresh metrics
    onClick(el.refreshMetricsBtn, async () => {
      if (!state.selectedRunId) return;
      try {
        state.metricsGrouped = await api(`/api/v1/runs/${state.selectedRunId}/metrics`);
        state.latestMetrics = await api(`/api/v1/runs/${state.selectedRunId}/metrics/latest`);
        renderMetrics();
        renderCharts();
        toast("Metrics refreshed");
      } catch (e) { toast(e.message, true); }
    });

    // Artifact upload
    onChange(el.artifactFileInput, () => {
      const file = el.artifactFileInput.files[0];
      if (file) uploadArtifact(file);
    });

    // Members
    onSubmit(el.memberForm, async (ev) => {
      ev.preventDefault();
      if (!state.selectedProjectId) { toast("Select a project first", true); return; }
      try {
        await api(`/api/v1/projects/${state.selectedProjectId}/members`, {
          method: "POST",
          body: JSON.stringify({ email: el.memberEmail.value.trim(), role: el.memberRole.value }),
        });
        el.memberForm.reset();
        await loadMembers();
        toast("Member added");
      } catch (e) { toast(e.message, true); }
    });

    // API Keys
    onSubmit(el.apiKeyForm, async (ev) => {
      ev.preventDefault();
      try {
        const payload = { name: el.apiKeyName.value.trim() };
        const expiry = el.apiKeyExpiry.value.trim();
        if (expiry) payload.expires_in = Number(expiry);
        const created = await api("/api/v1/api-keys", { method: "POST", body: JSON.stringify(payload) });
        el.apiKeyForm.reset();
        el.newKeyBox.classList.remove("hidden");
        el.newKeyValue.textContent = created.key;
        await loadApiKeys();
        toast("API key created");
      } catch (e) { toast(e.message, true); }
    });

    onClick(el.copyKeyBtn, () => {
      navigator.clipboard.writeText(el.newKeyValue.textContent).then(() => toast("Copied!"));
    });
  }

  /* ------------------------------------------------------------------ */
  /*  Init                                                               */
  /* ------------------------------------------------------------------ */
  async function init() {
    handleOAuthCallback();
    initTheme();
    bindEvents();
    if (state.token) {
      await bootstrapAuthed();
    } else {
      // Probe for DEV_NOAUTH_EMAIL mode: if /auth/me succeeds without a token
      // the server is running with auth bypassed — skip the login screen.
      try {
        const probe = await fetch("/api/v1/auth/me");
        if (probe.ok) {
          state.noauth = true;
          await bootstrapAuthed();
          return;
        }
      } catch { /* network error — fall through to login */ }
      setAuthUi(false);
    }
  }

  init();
})();

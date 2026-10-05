(() => {
  const SAMPLE_RATE = 16000;
  const START_RMS = 0.018;
  const STOP_RMS = 0.01;
  const SILENCE_MS = 450;
  const MIN_MS = 400;
  const MAX_MS = 12000;

  const $ = (id) => document.getElementById(id);
  const btnAsk = $("btnAsk");
  const btnPlus = $("btnPlus");
  const composer = $("composer");
  const hint = $("hint");
  const query = $("query");
  const emptyHero = $("emptyHero");
  const composerDot = $("composerDot");
  const agentPicker = $("agentPicker");
  const statusCard = $("statusCard");
  const listenState = $("listenState");
  const latencyText = $("latencyText");
  const dot = $("dot");
  const meterBar = $("meterBar");
  const eventLog = $("eventLog");
  const agentFilters = $("agentFilters");
  const skillFilters = $("skillFilters");
  const chatBoard = $("chatBoard");
  const chatThread = $("chatThread");
  const agentResult = $("agentResult");
  const resultLabel = $("resultLabel");
  const resultPrompt = $("resultPrompt");
  let resultSteps = $("resultSteps");
  let resultImage = $("resultImage");
  let resultEmpty = $("resultEmpty");
  let resultText = $("resultText");
  let currentTurn = null;
  let captionEl = null;
  let downloadEl = null;
  let workRail = null;
  let workLine = null;
  let workLabel = null;
  let workTime = null;
  let workLog = null;
  let thoughtRail = null;
  let thoughtLine = null;
  let thoughtLabel = null;
  let thoughtTime = null;
  let thoughtBody = null;
  let turnStartedAt = 0;
  let thinkingStartedAt = 0;
  let thinkingMs = 0;
  let tickTimer = null;

  const FALLBACK_AGENT = {
    id: "brand-agent",
    name: "品牌 Agent",
    skill: "poster-workflow",
    skills: [
      { id: "poster-workflow", name: "海报工作流" },
      { id: "brand-tone", name: "品牌调性" },
    ],
  };
  const AGENT_KEY = "movieagent.voice.agent";
  const SKILL_KEY = "movieagent.voice.skill";
  const SESSION_KEY = "movieagent.robot.session";
  const SESSION_META_KEY = "movieagent.robot.session.meta";

  let selectedAgent = FALLBACK_AGENT;
  let selectedSkill = FALLBACK_AGENT.skills[0];
  let listening = false;
  let starting = false;
  let speaking = false;
  let busy = false;
  let ctx = null;
  let stream = null;
  let processor = null;
  let source = null;
  let pcm = [];
  let speakStartedAt = 0;
  let lastVoiceAt = 0;
  let meterTimer = null;

  function log(msg) {
    const li = document.createElement("li");
    li.textContent = `[${new Date().toLocaleTimeString()}] ${msg}`;
    eventLog.prepend(li);
    while (eventLog.children.length > 40) eventLog.lastChild.remove();
  }

  function setUI(state, detail = "") {
    listenState.textContent = detail || state;
    composer.classList.toggle("listening", listening || starting);
    btnAsk.classList.toggle("active", listening && !speaking);
    btnAsk.classList.toggle("speaking", speaking);
    btnAsk.setAttribute("aria-pressed", listening ? "true" : "false");

    if (composerDot) {
      composerDot.className = "composer-dot";
      if (state === "err") composerDot.classList.add("err");
      else if (state === "warn" || starting || busy) composerDot.classList.add("warn");
      else if (listening) composerDot.classList.add("on");
    }

    const label = btnAsk.querySelector(".ask-text");
    if (label) {
      if (starting) label.textContent = "开麦中…";
      else if (listening) label.textContent = "结束转写";
      else if (query.value.trim()) label.textContent = "发送";
      else label.textContent = "开麦转写";
    }
    hint.textContent = listening
      ? `正在听 ${selectedAgent.name} / ${selectedSkillName()}：停顿后识别`
      : `当前：${selectedAgent.name} · ${selectedSkillName()}`;
    syncSendButton();
  }

  function syncSendButton() {
    const has = query.value.trim().length > 0;
    const sending = has && !listening && !starting && !busy;
    const mic = btnAsk.querySelector("[data-role=icon-mic]");
    const send = btnAsk.querySelector("[data-role=icon-send]");
    const stop = btnAsk.querySelector("[data-role=icon-stop]");
    if (mic) mic.hidden = listening || starting || sending;
    if (send) send.hidden = !sending;
    if (stop) stop.hidden = !(listening || starting);
    btnAsk.classList.toggle("is-send", sending);
    btnAsk.disabled = busy && !listening;
    btnAsk.setAttribute(
      "aria-label",
      listening || starting ? "结束转写" : sending ? "发送" : "开麦转写"
    );
  }

  function selectedAgentId() {
    return selectedAgent?.id || FALLBACK_AGENT.id;
  }

  function selectedSkillId() {
    return selectedSkill?.id || selectedAgent?.skill || FALLBACK_AGENT.skill;
  }

  function selectedSkillName() {
    return selectedSkill?.name || selectedSkillId();
  }

  function normalizeAgent(raw) {
    const skills = (raw.skills || []).map((s) => ({
      id: s.id || s.name,
      name: s.name || s.id,
    })).filter((s) => s.id);
    if (!skills.length && raw.skill) {
      skills.push({ id: raw.skill, name: raw.skill });
    }
    return {
      id: raw.id,
      name: raw.name || raw.id,
      skill: raw.skill || (skills[0] && skills[0].id) || "",
      skills,
    };
  }

  function pickSkill(agent, preferred) {
    const skills = agent.skills || [];
    if (preferred && skills.some((s) => s.id === preferred)) {
      return skills.find((s) => s.id === preferred);
    }
    if (agent.skill && skills.some((s) => s.id === agent.skill)) {
      return skills.find((s) => s.id === agent.skill);
    }
    return skills[0] || { id: agent.skill || "poster-workflow", name: "海报工作流" };
  }

  function renderAgentFilters(agents) {
    const seen = new Set();
    const list = [];
    const push = (raw) => {
      const a = normalizeAgent(raw);
      if (!a.id || seen.has(a.id)) return;
      seen.add(a.id);
      list.push(a);
    };
    for (const a of agents || []) push(a);
    if (!list.length) push(FALLBACK_AGENT);
    const saved = localStorage.getItem(AGENT_KEY);
    selectedAgent = (saved && list.find((a) => a.id === saved)) || list[0];
    selectedSkill = pickSkill(selectedAgent, localStorage.getItem(SKILL_KEY));
    agentFilters.innerHTML = "";
    for (const a of list) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "agent-chip" + (a.id === selectedAgent.id ? " active" : "");
      btn.setAttribute("role", "option");
      btn.setAttribute("aria-selected", a.id === selectedAgent.id ? "true" : "false");
      btn.dataset.agentId = a.id;
      btn.textContent = a.name;
      btn.addEventListener("click", () => {
        selectedAgent = a;
        localStorage.setItem(AGENT_KEY, a.id);
        selectedSkill = pickSkill(a, selectedSkill && a.skills.some((s) => s.id === selectedSkill.id) ? selectedSkill.id : a.skill);
        localStorage.setItem(SKILL_KEY, selectedSkill.id);
        sessionStorage.removeItem(SESSION_KEY);
        sessionStorage.removeItem(SESSION_META_KEY);
        agentFilters.querySelectorAll(".agent-chip").forEach((el) => {
          const on = el.dataset.agentId === a.id;
          el.classList.toggle("active", on);
          el.setAttribute("aria-selected", on ? "true" : "false");
        });
        renderSkillFilters(a.skills);
        setUI(listening ? "on" : "", listening ? `已开麦 · ${a.name}` : "未开麦");
        log(`已选择 Agent ${a.name}`);
      });
      agentFilters.appendChild(btn);
    }
    renderSkillFilters(selectedAgent.skills);
    setUI(listening ? "on" : "", listening ? `已开麦 · ${selectedAgent.name}` : "未开麦");
  }

  function renderSkillFilters(skills) {
    if (!skillFilters) return;
    skillFilters.innerHTML = "";
    const list = skills && skills.length ? skills : [selectedSkill];
    for (const s of list) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "agent-chip skill-chip" + (s.id === selectedSkillId() ? " active" : "");
      btn.setAttribute("role", "option");
      btn.setAttribute("aria-selected", s.id === selectedSkillId() ? "true" : "false");
      btn.dataset.skillId = s.id;
      btn.textContent = s.name || s.id;
      btn.addEventListener("click", () => {
        selectedSkill = s;
        localStorage.setItem(SKILL_KEY, s.id);
        sessionStorage.removeItem(SESSION_KEY);
        sessionStorage.removeItem(SESSION_META_KEY);
        skillFilters.querySelectorAll(".skill-chip").forEach((el) => {
          const on = el.dataset.skillId === s.id;
          el.classList.toggle("active", on);
          el.setAttribute("aria-selected", on ? "true" : "false");
        });
        setUI(listening ? "on" : "", listening ? `已开麦 · ${selectedSkillName()}` : "未开麦");
        log(`已选择 Skill ${s.name || s.id}`);
      });
      skillFilters.appendChild(btn);
    }
  }

  function enterChat() {
    document.body.classList.add("is-chatting");
    if (emptyHero) emptyHero.hidden = true;
    if (chatBoard) chatBoard.hidden = false;
    query.placeholder = "继续问，消息会排队发送...";
    if (agentPicker) agentPicker.hidden = true;
    if (btnPlus) btnPlus.setAttribute("aria-expanded", "false");
  }

  function escapeHtml(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function formatClock(d) {
    const hh = String(d.getHours()).padStart(2, "0");
    const mm = String(d.getMinutes()).padStart(2, "0");
    return `今天 ${hh}:${mm}`;
  }

  function appendUserBubble(text) {
    enterChat();
    const art = document.createElement("article");
    art.className = "chat-turn user";
    art.innerHTML =
      `<div class="bubble">${escapeHtml(text)}</div>`;
    chatThread.appendChild(art);
    art.scrollIntoView({ behavior: "smooth", block: "end" });
  }

  function foldMarkup(role, label) {
    return (
      `<details class="cursor-fold" data-role="${role}">` +
        `<summary class="fold-summary">` +
          `<span class="fold-live" data-role="${role}-line">` +
            `<span class="fold-label" data-role="${role}-label">${label}</span>` +
            `<span class="fold-time" data-role="${role}-time"></span>` +
            `<span class="fold-chevron" aria-hidden="true"></span>` +
          `</span>` +
        `</summary>` +
        (role === "thought"
          ? `<p class="thought-body" data-role="thought-body"></p>`
          : `<ul class="work-log" data-role="work-log"></ul>`) +
      `</details>`
    );
  }

  function appendAssistantTurn() {
    enterChat();
    const art = document.createElement("article");
    art.className = "chat-turn assistant";
    art.innerHTML =
      `<div class="assistant-body">` +
        foldMarkup("work", "Working") +
        foldMarkup("thought", "Thinking") +
        `<p class="assistant-text pending" data-role="text" hidden>正在为你生成…</p>` +
        `<figure class="chat-card" data-role="card" hidden>` +
          `<div class="chat-card-stage">` +
            `<img data-role="image" alt="生成结果" hidden />` +
            `<a class="chat-download" data-role="download" hidden download="poster.png" aria-label="下载图片">` +
              `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">` +
                `<path d="M12 4v10m0 0 4-4m-4 4-4-4" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round"/>` +
                `<path d="M5 18h14" stroke="currentColor" stroke-width="1.9" stroke-linecap="round"/>` +
              `</svg>` +
            `</a>` +
          `</div>` +
        `</figure>` +
        `<p class="assistant-text caption" data-role="caption" hidden></p>` +
      `</div>`;
    chatThread.appendChild(art);
    bindTurn(art);
    thoughtRail.hidden = true;
    turnStartedAt = Date.now();
    thinkingStartedAt = 0;
    thinkingMs = 0;
    resultEmpty = null;
    setWorkLive("Working", true);
    startTick();
    art.scrollIntoView({ behavior: "smooth", block: "end" });
    return art;
  }

  function bindTurn(art) {
    currentTurn = art;
    resultImage = art.querySelector("[data-role=image]");
    resultText = art.querySelector("[data-role=text]");
    captionEl = art.querySelector("[data-role=caption]");
    downloadEl = art.querySelector("[data-role=download]");
    workRail = art.querySelector("[data-role=work]");
    workLine = art.querySelector("[data-role=work-line]");
    workLabel = art.querySelector("[data-role=work-label]");
    workTime = art.querySelector("[data-role=work-time]");
    workLog = art.querySelector("[data-role=work-log]");
    thoughtRail = art.querySelector("[data-role=thought]");
    thoughtLine = art.querySelector("[data-role=thought-line]");
    thoughtLabel = art.querySelector("[data-role=thought-label]");
    thoughtTime = art.querySelector("[data-role=thought-time]");
    thoughtBody = art.querySelector("[data-role=thought-body]");
    resultSteps = null;
  }

  function formatDuration(ms) {
    const sec = Math.max(0, Math.round(ms / 1000));
    if (sec < 60) return `${sec}s`;
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return s ? `${m}m ${s}s` : `${m}m`;
  }

  function startTick() {
    stopTick();
    tickTimer = setInterval(() => {
      if (workRail && workRail.classList.contains("is-running") && workTime && turnStartedAt) {
        workTime.textContent = formatDuration(Date.now() - turnStartedAt);
      }
      if (thoughtRail && thoughtRail.classList.contains("is-running") && thoughtTime && thinkingStartedAt) {
        thoughtTime.textContent = formatDuration(Date.now() - thinkingStartedAt + thinkingMs);
      }
    }, 1000);
  }

  function stopTick() {
    if (tickTimer) {
      clearInterval(tickTimer);
      tickTimer = null;
    }
  }

  function swapLine(el) {
    if (!el) return;
    el.classList.remove("is-swap");
    void el.offsetWidth;
    el.classList.add("is-swap");
  }

  function setWorkLive(label, running) {
    if (!workLabel) return;
    if (workRail) workRail.classList.toggle("is-running", !!running);
    swapLine(workLine);
    workLabel.textContent = label;
    if (running && workTime && turnStartedAt) {
      workTime.textContent = formatDuration(Date.now() - turnStartedAt);
    }
  }

  function setThoughtLive(label, running) {
    if (!thoughtLabel) return;
    thoughtRail.hidden = false;
    thoughtRail.classList.toggle("is-running", !!running);
    thoughtRail.open = !!running;
    swapLine(thoughtLine);
    thoughtLabel.textContent = label;
    if (running && thoughtTime) {
      const ms = (thinkingStartedAt ? Date.now() - thinkingStartedAt : 0) + thinkingMs;
      thoughtTime.textContent = formatDuration(ms || 0);
    }
  }

  function pushWorkItem(kind, text, state) {
    if (!workLog || !text) return;
    const li = document.createElement("li");
    li.className = `work-item ${kind}${state ? " " + state : ""}`;
    li.innerHTML = `<span class="work-kind">${escapeHtml(kind)}</span><span class="work-msg">${escapeHtml(text)}</span>`;
    workLog.appendChild(li);
    while (workLog.children.length > 40) workLog.firstChild.remove();
    const last = workLog.lastElementChild;
    if (last) last.scrollIntoView({ block: "nearest" });
  }

  function appendThought(delta) {
    if (!thoughtBody || !delta) return;
    const line = String(delta).trim();
    if (!line) return;
    const prev = thoughtBody.textContent ? thoughtBody.textContent.trim() : "";
    if (prev && (prev.endsWith(line) || prev.includes(line))) return;
    thoughtBody.textContent = prev ? prev + "\n" + line : line;
    thoughtBody.scrollTop = thoughtBody.scrollHeight;
  }

  function settleThought() {
    if (thinkingStartedAt) {
      thinkingMs += Date.now() - thinkingStartedAt;
      thinkingStartedAt = 0;
    }
    if (!thoughtRail || thoughtRail.hidden) return;
    thoughtRail.classList.remove("is-running");
    thoughtRail.open = false;
    if (thoughtLabel) thoughtLabel.textContent = "Thought";
    if (thoughtTime) thoughtTime.textContent = formatDuration(thinkingMs || 1);
  }

  function settleTrace() {
    settleThought();
    stopTick();
    if (workRail) {
      workRail.classList.remove("is-running");
      workRail.open = false;
    }
    if (workLabel) workLabel.textContent = "Worked for";
    if (workTime && turnStartedAt) {
      workTime.textContent = formatDuration(Date.now() - turnStartedAt);
    }
  }

  async function submitText() {
    const text = query.value.trim();
    if (!text || busy) return;
    if (listening || starting) stop();
    busy = true;
    syncSendButton();
    try {
      await speakToAgent(text);
      setUI("", "未开麦");
    } catch (err) {
      setUI("err", err.message || String(err));
      log(`失败: ${err.message || err}`);
    } finally {
      busy = false;
      syncSendButton();
    }
  }

  async function ensureRobotSession(agentId, skillId) {
    const meta = `${agentId || "brand-agent"}|${skillId || ""}`;
    let id = sessionStorage.getItem(SESSION_KEY) || "";
    if (id && sessionStorage.getItem(SESSION_META_KEY) === meta) {
      const probe = await fetch("/api/v1/sessions/" + encodeURIComponent(id), { cache: "no-store" });
      if (probe.ok) return id;
    }
    const res = await fetch("/api/v1/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent_id: agentId || "brand-agent", skill: skillId || "" }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok || body.code !== 0 || !body.data?.session_id) {
      throw new Error(body.message || "无法开启 robot session");
    }
    id = body.data.session_id;
    sessionStorage.setItem(SESSION_KEY, id);
    sessionStorage.setItem(SESSION_META_KEY, meta);
    log(`已开启 session ${id} · ${meta}`);
    return id;
  }

  async function speakToAgent(text) {
    const agentId = selectedAgentId();
    const name = selectedAgent.name;
    log(`交给 ${name} / ${selectedSkillName()}：${text}`);
    setUI("warn", `${selectedSkillName()} 处理中…`);
    appendUserBubble(text);
    appendAssistantTurn();
    query.value = "";
    syncSendButton();
    const sessionId = await ensureRobotSession(agentId, selectedSkillId());
    const res = await fetch("/api/v1/sessions/" + encodeURIComponent(sessionId) + "/messages", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/x-ndjson" },
      body: JSON.stringify({ agent_id: agentId, skill: selectedSkillId(), prompt: text }),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      showResultError(name, body.message || `${name} 暂未接入`);
      throw new Error(body.message || `${name} 暂未接入`);
    }
    const data = await consumeRobotStream(res, name, text);
    if (data && data.error) {
      showResultError(name, data.error);
      throw new Error(data.error);
    }
    if (data) {
      renderAgentResult(name, text, data);
      log(data.message || `${name} 已完成`);
    }
  }

  async function consumeRobotStream(res, name, prompt) {
    const reader = res.body && res.body.getReader ? res.body.getReader() : null;
    if (!reader) {
      const body = await res.json().catch(() => ({}));
      return body.data || body;
    }
    const dec = new TextDecoder();
    let buf = "";
    let lastData = null;
    while (true) {
      const { done, value } = await reader.read();
      buf += dec.decode(value || new Uint8Array(), { stream: !done });
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl).trim();
        buf = buf.slice(nl + 1);
        if (!line) continue;
        let ev;
        try {
          ev = JSON.parse(line);
        } catch {
          continue;
        }
        applyRobotEvent(ev, name, prompt);
        if (ev.type === "agent_end") lastData = ev.data || lastData;
      }
      if (done) break;
    }
    return lastData;
  }

  function setStatusLine(text, pending) {
    if (!resultText) return;
    if (text) {
      resultText.hidden = false;
      resultText.textContent = text;
    }
    resultText.classList.toggle("pending", !!pending);
  }

  function applyRobotEvent(ev, name, prompt) {
    if (!ev || !ev.type) return;
    if (ev.type === "agent_start") {
      setWorkLive("Working", true);
      pushWorkItem("work", "started");
      log("agent_start · robot session");
    } else if (ev.type === "turn_start") {
      setWorkLive("Working", true);
      log("turn_start");
    } else if (ev.type === "thinking_start") {
      thinkingStartedAt = Date.now();
      setWorkLive("Working", false);
      setThoughtLive("Thinking", true);
      appendThought(ev.thinking || ev.delta || "理解需求…");
      log("thinking_start");
    } else if (ev.type === "thinking_delta") {
      const line = String(ev.delta || ev.thinking || "").trim();
      if (line) {
        if (thoughtRail && !thoughtRail.classList.contains("is-running")) {
          thinkingStartedAt = Date.now();
          setThoughtLive("Thinking", true);
        }
        appendThought(line);
      }
    } else if (ev.type === "thinking_end") {
      settleThought();
      log("thinking_end");
    } else if (ev.type === "tool_execution_start") {
      const tool = ev.toolName || ev.toolCallId || "tool";
      settleThought();
      setWorkLive(tool, true);
      pushWorkItem("tool", tool, "run");
      log(`tool start · ${tool}`);
    } else if (ev.type === "tool_execution_end") {
      const tool = ev.toolName || ev.toolCallId || "tool";
      const err = !!ev.isError;
      pushWorkItem("tool", tool, err ? "err" : "ok");
      setWorkLive(err ? `${tool} failed` : tool, true);
      log(`tool end · ${tool}${err ? " 失败" : ""}`);
    } else if (ev.type === "skill_start") {
      const skill = ev.toolName || ev.skill || "skill";
      settleThought();
      setWorkLive(skill, true);
      pushWorkItem("skill", skill, "run");
      log(`skill start · ${skill}`);
    } else if (ev.type === "skill_end") {
      const skill = ev.toolName || ev.skill || "skill";
      const err = !!ev.isError;
      pushWorkItem("skill", skill, err ? "err" : "ok");
      setWorkLive(skill, true);
      log(`skill end · ${skill}${err ? " 失败" : ""}`);
    } else if (ev.type === "message_end" && ev.message && ev.message.role === "assistant" && ev.message.content) {
      setStatusLine(ev.message.content, false);
    } else if (ev.type === "agent_end" && ev.data && !ev.data.error) {
      renderAgentResult(name, prompt, ev.data);
    } else if (ev.type === "agent_end" && ev.data && ev.data.error) {
      showResultError(name, ev.data.error);
    } else if (ev.type === "agent_settled") {
      settleTrace();
      log("agent_settled");
    }
  }

  function showResultError(name, message) {
    if (!currentTurn) appendAssistantTurn();
    settleTrace();
    setStatusLine(message || "执行失败", false);
    pushWorkItem("work", message || "失败", "err");
  }

  function renderAgentResult(name, prompt, data) {
    if (!currentTurn) appendAssistantTurn();
    if (resultLabel) resultLabel.textContent = name;
    if (resultPrompt) resultPrompt.textContent = prompt || data.prompt || "";
    settleTrace();
    const text = data.message || data.text || data.output || "";
    if (text) setStatusLine(text, false);
    else if (resultText && resultText.classList.contains("pending")) {
      resultText.hidden = true;
    }

    const caption = data.creative_text || "";
    if (captionEl && caption && caption !== text) {
      captionEl.hidden = false;
      captionEl.textContent = caption;
    }

    const imageURL = data.image_url || data.image || "";
    const card = currentTurn && currentTurn.querySelector("[data-role=card]");
    if (imageURL && resultImage) {
      if (card) {
        card.hidden = false;
        card.classList.add("is-ready");
      }
      resultImage.hidden = true;
      resultImage.onload = () => {
        resultImage.hidden = false;
        if (card) card.classList.add("is-loaded");
      };
      resultImage.onerror = () => {
        resultImage.hidden = true;
        if (resultText) resultText.textContent = (resultText.textContent || "") + "\n图片加载失败";
      };
      resultImage.src = imageURL + (imageURL.includes("?") ? "&" : "?") + "t=" + Date.now();
      bindDownload(imageURL);
    }
  }

  function bindDownload(url) {
    if (!downloadEl || !url) return;
    downloadEl.hidden = false;
    downloadEl.href = url;
    downloadEl.onclick = async (e) => {
      e.preventDefault();
      try {
        const res = await fetch(url);
        const blob = await res.blob();
        const obj = URL.createObjectURL(blob);
        const tmp = document.createElement("a");
        tmp.href = obj;
        tmp.download = "poster.png";
        document.body.appendChild(tmp);
        tmp.click();
        tmp.remove();
        URL.revokeObjectURL(obj);
      } catch {
        window.open(url, "_blank");
      }
    };
  }

  function rmsOf(buf) {
    let sum = 0;
    for (let i = 0; i < buf.length; i++) sum += buf[i] * buf[i];
    return Math.sqrt(sum / Math.max(1, buf.length));
  }

  function downsample(input, fromRate) {
    if (fromRate === SAMPLE_RATE) return new Float32Array(input);
    const ratio = fromRate / SAMPLE_RATE;
    const outLen = Math.max(1, Math.round(input.length / ratio));
    const out = new Float32Array(outLen);
    for (let i = 0; i < outLen; i++) {
      out[i] = input[Math.min(input.length - 1, Math.floor(i * ratio))] || 0;
    }
    return out;
  }

  function encodeWav(float32) {
    const pcm16 = new Int16Array(float32.length);
    for (let i = 0; i < float32.length; i++) {
      const s = Math.max(-1, Math.min(1, float32[i]));
      pcm16[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
    }
    const bytes = pcm16.byteLength;
    const buf = new ArrayBuffer(44 + bytes);
    const view = new DataView(buf);
    const writeStr = (offset, str) => {
      for (let i = 0; i < str.length; i++) view.setUint8(offset + i, str.charCodeAt(i));
    };
    writeStr(0, "RIFF");
    view.setUint32(4, 36 + bytes, true);
    writeStr(8, "WAVE");
    writeStr(12, "fmt ");
    view.setUint32(16, 16, true);
    view.setUint16(20, 1, true);
    view.setUint16(22, 1, true);
    view.setUint32(24, SAMPLE_RATE, true);
    view.setUint32(28, SAMPLE_RATE * 2, true);
    view.setUint16(32, 2, true);
    view.setUint16(34, 16, true);
    writeStr(36, "data");
    view.setUint32(40, bytes, true);
    new Uint8Array(buf, 44).set(new Uint8Array(pcm16.buffer));
    return new Blob([buf], { type: "audio/wav" });
  }

  async function transcribeBlob(blob) {
    busy = true;
    setUI("warn", "识别中…");
    const t0 = performance.now();
    const fd = new FormData();
    fd.append("file", blob, "utterance.wav");
    const res = await fetch("/api/v1/voice/transcribe", { method: "POST", body: fd });
    const body = await res.json().catch(() => ({}));
    const ms = Math.round(performance.now() - t0);
    if (!res.ok || body.code !== 0) {
      busy = false;
      throw new Error(body.message || res.statusText);
    }
    latencyText.textContent = `识别 ${body.data?.ms || ms}ms`;
    const text = (body.data?.text || "").trim();
    if (!text) {
      busy = false;
      log("这一句没有识别出文字");
      return;
    }
    query.value = text;
    syncSendButton();
    log(`已识别：${text}`);
    try {
      await speakToAgent(text);
    } finally {
      busy = false;
      syncSendButton();
    }
  }

  async function flushUtterance() {
    if (!pcm.length) return;
    const merged = concat(pcm);
    pcm = [];
    speaking = false;
    const dur = merged.length / SAMPLE_RATE;
    if (dur * 1000 < MIN_MS) {
      log("太短，已忽略");
      setUI("on", `已开麦 · ${selectedAgent.name}`);
      return;
    }
    try {
      await transcribeBlob(encodeWav(merged));
      if (listening) setUI("on", `已开麦 · ${selectedAgent.name}`);
    } catch (err) {
      setUI("err", err.message || String(err));
      log(`失败: ${err.message || err}`);
      if (listening) setUI("on", `已开麦 · ${selectedAgent.name}`);
    }
  }

  function concat(chunks) {
    let n = 0;
    for (const c of chunks) n += c.length;
    const out = new Float32Array(n);
    let o = 0;
    for (const c of chunks) {
      out.set(c, o);
      o += c.length;
    }
    return out;
  }

  function onAudio(evt) {
    const input = evt.inputBuffer.getChannelData(0);
    const rms = rmsOf(input);
    meterBar.style.width = `${Math.min(100, Math.round(rms * 400))}%`;
    const now = performance.now();
    const down = downsample(input, ctx.sampleRate);

    if (!speaking && rms >= START_RMS && !busy) {
      speaking = true;
      speakStartedAt = now;
      lastVoiceAt = now;
      pcm = [down];
      setUI("speaking", "你正在说…");
      log("开始说话");
      return;
    }
    if (speaking) {
      pcm.push(down);
      if (rms >= STOP_RMS) lastVoiceAt = now;
      const elapsed = now - speakStartedAt;
      const silent = now - lastVoiceAt;
      if (elapsed >= MAX_MS || silent >= SILENCE_MS) {
        flushUtterance();
      }
    }
  }

  async function start() {
    if (listening || starting) return;
    starting = true;
    setUI("warn", "请求麦克风…");
    try {
      const probe = await fetch("/api/v1/voice/status", { cache: "no-store" });
      const body = await probe.json().catch(() => ({}));
      if (!probe.ok || body.code !== 0) {
        throw new Error(body.message || "无法连接 movieagent");
      }
      if (!body.data?.ready) {
        throw new Error(body.data?.message || "本地 Whisper 未就绪");
      }
      log(`Whisper 就绪 · ${body.data.model}`);
      stream = await navigator.mediaDevices.getUserMedia({
        audio: { echoCancellation: true, noiseSuppression: true, channelCount: 1 },
      });
      ctx = new AudioContext();
      source = ctx.createMediaStreamSource(stream);
      processor = ctx.createScriptProcessor(4096, 1, 1);
      processor.onaudioprocess = onAudio;
      const mute = ctx.createGain();
      mute.gain.value = 0;
      source.connect(processor);
      processor.connect(mute);
      mute.connect(ctx.destination);
      listening = true;
      starting = false;
      setUI("on", `已开麦 · ${selectedAgent.name}`);
      log(`麦克风已打开，对 ${selectedAgent.name} 说话`);
    } catch (err) {
      starting = false;
      stop();
      setUI("err", err.message || String(err));
      log(`开麦失败: ${err.message || err}`);
    }
  }

  function stop() {
    listening = false;
    starting = false;
    speaking = false;
    pcm = [];
    if (processor) {
      processor.onaudioprocess = null;
      processor.disconnect();
      processor = null;
    }
    if (source) {
      source.disconnect();
      source = null;
    }
    if (ctx) {
      ctx.close().catch(() => {});
      ctx = null;
    }
    if (stream) {
      stream.getTracks().forEach((t) => t.stop());
      stream = null;
    }
    if (meterTimer) clearInterval(meterTimer);
    meterTimer = null;
    meterBar.style.width = "0%";
    setUI("", "未开麦");
  }

  btnAsk.addEventListener("click", () => {
    if (listening || starting) stop();
    else if (query.value.trim()) submitText();
    else start();
  });

  if (btnPlus && agentPicker) {
    btnPlus.addEventListener("click", () => {
      const open = agentPicker.hidden;
      agentPicker.hidden = !open;
      btnPlus.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  composer.addEventListener("submit", (e) => {
    e.preventDefault();
    submitText();
  });
  query.addEventListener("input", syncSendButton);

  fetch("/api/v1/agents")
    .then((r) => r.json())
    .then((body) => renderAgentFilters(body.data?.agents || []))
    .catch(() => renderAgentFilters([]));

  fetch("/api/v1/voice/transcripts")
    .then((r) => r.json())
    .then((body) => {
      const collected = body.data?.collected || [];
      if (collected.length) {
        query.value = collected[collected.length - 1].text;
        syncSendButton();
      }
    })
    .catch(() => {});

  fetch("/api/v1/voice/status", { cache: "no-store" })
    .then((r) => r.json())
    .then((body) => {
      if (body.data?.ready) {
        log(`Whisper 就绪 · ${body.data.model}`);
        setUI("", "未开麦");
      } else {
        const msg = body.data?.message || body.message || "Whisper 未就绪";
        log(msg);
        setUI("warn", msg);
      }
    })
    .catch((e) => {
      log(`无法连接 movieagent: ${e.message || e}`);
      setUI("err", "movieagent 未运行");
    });

  setUI("", "未开麦");
})();

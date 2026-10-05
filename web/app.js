(() => {
  const STEPS = [
    { id: "ingest", name: "产品图" },
    { id: "brief", name: "简报" },
    { id: "creative_design", name: "创意" },
    { id: "prompt", name: "Prompt" },
    { id: "render", name: "出图" },
    { id: "brand_composite", name: "品牌整合" },
  ];

  const state = { uploads: [], elements: [], busy: false };

  const els = {
    prompt: document.getElementById("prompt"),
    brand: document.getElementById("brand"),
    drop: document.getElementById("dropzone"),
    file: document.getElementById("file-input"),
    preview: document.getElementById("preview"),
    dropLabel: document.getElementById("drop-label"),
    elements: document.getElementById("elements"),
    elementInput: document.getElementById("element-input"),
    addElement: document.getElementById("btn-element"),
    send: document.getElementById("btn-send"),
    demo: document.getElementById("btn-demo"),
    status: document.getElementById("status"),
    steps: document.getElementById("steps"),
    image: document.getElementById("result-image"),
    empty: document.getElementById("empty"),
    creative: document.getElementById("creative"),
    path: document.getElementById("path"),
  };

  function setStatus(text, isError) {
    els.status.textContent = text;
    els.status.classList.toggle("error", !!isError);
  }

  function renderSteps(mode, nodes) {
    const byId = {};
    for (const n of nodes || []) byId[n.id] = n;
    els.steps.innerHTML = "";
    for (const step of STEPS) {
      const li = document.createElement("li");
      const node = byId[step.id];
      let cls = "";
      if (mode === "run") cls = "run";
      else if (node) cls = node.status === "ok" ? "ok" : node.status === "error" ? "err" : "";
      li.className = cls;
      li.innerHTML = `<i></i>${step.name}`;
      els.steps.appendChild(li);
    }
  }

  async function api(path, opts) {
    const res = await fetch(path, opts);
    const body = await res.json();
    if (!res.ok || body.code !== 0) {
      throw new Error(body.message || res.statusText);
    }
    return body.data;
  }

  async function loadBrands() {
    const data = await api("/api/v1/brands");
    const brands = data.brands || [];
    els.brand.innerHTML = '<option value="">不使用品牌 Logo</option>';
    for (const b of brands) {
      const opt = document.createElement("option");
      opt.value = b.id;
      opt.textContent = b.name;
      els.brand.appendChild(opt);
    }
    if ([...els.brand.options].some((o) => o.value === "ningdao")) {
      els.brand.value = "ningdao";
    }
  }

  function showLocalPreview(file) {
    const url = URL.createObjectURL(file);
    els.preview.hidden = false;
    els.preview.src = url;
    els.dropLabel.hidden = true;
  }

  async function uploadFiles(fileList) {
    const files = [...fileList].filter((f) => (f.type || "").startsWith("image/"));
    if (!files.length) {
      setStatus("请上传图片", true);
      return;
    }
    setStatus("正在上传产品图…");
    const file = files[0];
    const fd = new FormData();
    fd.append("file", file);
    fd.append("kind", "image");
    if (els.brand.value) fd.append("brand_id", els.brand.value);
    const rec = await api("/api/v1/assets/upload", { method: "POST", body: fd });
    state.uploads = [rec];
    showLocalPreview(file);
    setStatus("产品图已就绪");
  }

  function renderElements() {
    els.elements.innerHTML = "";
    for (const el of state.elements) {
      const li = document.createElement("li");
      li.className = "element";
      const img = document.createElement("img");
      img.alt = "";
      img.src = el.preview;
      const note = document.createElement("input");
      note.type = "text";
      note.placeholder = "备注，例如：放在右上角的桂花枝";
      note.value = el.note;
      note.addEventListener("input", () => {
        el.note = note.value;
      });
      const remove = document.createElement("button");
      remove.type = "button";
      remove.setAttribute("aria-label", "移除元素");
      remove.textContent = "×";
      remove.addEventListener("click", () => {
        state.elements = state.elements.filter((x) => x.key !== el.key);
        renderElements();
      });
      li.append(img, note, remove);
      els.elements.appendChild(li);
    }
  }

  async function addElements(fileList) {
    const files = [...fileList].filter((f) => (f.type || "").startsWith("image/"));
    if (!files.length) {
      setStatus("请上传图片元素", true);
      return;
    }
    const room = 6 - state.elements.length;
    if (room <= 0) {
      setStatus("最多 6 个元素", true);
      return;
    }
    setStatus("正在上传元素…");
    for (const file of files.slice(0, room)) {
      const fd = new FormData();
      fd.append("file", file);
      fd.append("kind", "image");
      if (els.brand.value) fd.append("brand_id", els.brand.value);
      const rec = await api("/api/v1/assets/upload", { method: "POST", body: fd });
      state.elements.push({
        key: rec.id,
        id: rec.id,
        note: "",
        preview: URL.createObjectURL(file),
      });
    }
    renderElements();
    setStatus("元素已添加，可填写备注");
  }

  function showResult(data) {
    renderSteps("done", data.nodes);
    if (data.creative_text) els.creative.textContent = data.creative_text;
    else els.creative.textContent = "（暂无创意内容输出）";
    els.path.textContent = data.output_path || "";
    if (data.image_url) {
      const join = data.image_url.includes("?") ? "&" : "?";
      els.image.onload = () => {
        els.empty.hidden = true;
        els.image.hidden = false;
      };
      els.image.onerror = () => {
        els.image.hidden = true;
        els.empty.hidden = false;
        els.empty.textContent = "海报加载失败";
      };
      els.empty.hidden = false;
      els.empty.textContent = "正在加载海报…";
      els.image.hidden = true;
      els.image.src = data.image_url + join + "t=" + Date.now();
    } else {
      els.image.hidden = true;
      els.empty.hidden = false;
      els.empty.textContent = "这次没有返回海报";
    }
    setStatus(data.message || "完成");
  }

  async function submitTask() {
    const prompt = els.prompt.value.trim();
    if (!prompt && !state.uploads.length && !state.elements.length) {
      setStatus("请填写需求，或先上传产品图 / 元素", true);
      return;
    }
    state.busy = true;
    els.send.disabled = true;
    els.demo.disabled = true;
    renderSteps("run");
    setStatus("正在执行品牌 Agent，通常需要约一分钟…");
    els.creative.textContent = "生成中…";
    els.image.hidden = true;
    els.image.removeAttribute("src");
    els.empty.hidden = false;
    els.empty.textContent = "正在生成海报…";
    try {
      const data = await api("/api/v1/tasks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          agent_id: "brand-agent",
          prompt,
          brand_id: els.brand.value || undefined,
          asset_ids: state.uploads.map((u) => u.id),
          elements: state.elements.map((el) => ({
            asset_id: el.id,
            note: el.note.trim(),
          })),
        }),
      });
      showResult(data);
    } catch (err) {
      renderSteps("done", STEPS.map((s) => ({ id: s.id, status: "error" })));
      setStatus(err.message || String(err), true);
      els.creative.textContent = "执行失败";
    } finally {
      state.busy = false;
      els.send.disabled = false;
      els.demo.disabled = false;
    }
  }

  els.drop.addEventListener("click", () => els.file.click());
  els.file.addEventListener("change", () => {
    uploadFiles(els.file.files).catch((err) => setStatus(err.message || String(err), true));
    els.file.value = "";
  });
  ["dragenter", "dragover"].forEach((ev) => {
    els.drop.addEventListener(ev, (e) => {
      e.preventDefault();
      els.drop.classList.add("dragover");
    });
  });
  ["dragleave", "drop"].forEach((ev) => {
    els.drop.addEventListener(ev, (e) => {
      e.preventDefault();
      els.drop.classList.remove("dragover");
    });
  });
  els.drop.addEventListener("drop", (e) => {
    uploadFiles(e.dataTransfer.files).catch((err) => setStatus(err.message || String(err), true));
  });
  els.addElement.addEventListener("click", () => els.elementInput.click());
  els.elementInput.addEventListener("change", () => {
    addElements(els.elementInput.files).catch((err) => setStatus(err.message || String(err), true));
    els.elementInput.value = "";
  });

  els.send.addEventListener("click", submitTask);
  els.prompt.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") submitTask();
  });
  els.demo.addEventListener("click", () => {
    els.prompt.value = "为中秋节柠檬茶新品做一张竖版海报，氛围团圆温馨，保留饮品主体。";
    setStatus("已填入中秋示例，可上传产品图后生成");
  });

  renderSteps("idle");
  loadBrands().catch((err) => setStatus(err.message || String(err), true));
})();

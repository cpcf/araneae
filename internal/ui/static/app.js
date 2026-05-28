const filterSelect = document.getElementById("filter-select");
const sortSelect = document.getElementById("sort-select");
const searchInput = document.getElementById("search-input");
const linksTbody = document.getElementById("links-tbody");
const skippedTbody = document.getElementById("skipped-tbody");
const linksView = document.getElementById("links-view");
const skippedView = document.getElementById("skipped-view");
const linksEmpty = document.getElementById("links-empty");
const skippedEmpty = document.getElementById("skipped-empty");
const detailEmpty = document.getElementById("detail-empty");
const detailContent = document.getElementById("detail-content");
const summary = document.getElementById("summary");
const viewLinksBtn = document.getElementById("view-links");
const viewSkippedBtn = document.getElementById("view-skipped");
const linksDetailURL = document.getElementById("detail-url");
const linksDetailFetch = document.getElementById("detail-fetch");
const linksDetailStatusCode = document.getElementById("detail-status-code");
const linksDetailStatus = document.getElementById("detail-status");
const linksDetailFinal = document.getElementById("detail-final");
const linksDetailProblem = document.getElementById("detail-problem");
const linksDetailError = document.getElementById("detail-error");
const linksDetailContentType = document.getElementById("detail-content-type");
const linksDetailRedirect = document.getElementById("detail-redirect");
const detailSources = document.getElementById("detail-sources");
const metricDiscovered = document.getElementById("metric-discovered");
const metricOccurrences = document.getElementById("metric-occurrences");
const metricDead = document.getElementById("metric-dead");
const metricNon200 = document.getElementById("metric-non200");
const metricSkipped = document.getElementById("metric-skipped");
const metricOk = document.getElementById("metric-ok");

let reportData = null;
let fetchesByURL = new Map();

const copySupport = Boolean(window.isSecureContext && navigator.clipboard && navigator.clipboard.writeText);

async function main() {
  try {
    const res = await fetch("/api/report");
    if (!res.ok) {
      throw new Error(`failed to load report (${res.status})`);
    }
    reportData = await res.json();
  } catch (err) {
    summary.textContent = `Could not load report: ${err.message}`;
    return;
  }

  reportData.links = reportData.links || [];
  reportData.fetches = reportData.fetches || [];
  reportData.skipped_links = reportData.skipped_links || [];

  fetchesByURL = new Map(reportData.fetches.map((f) => [f.url, f]));
  setMetrics();
  bindControls();
  setView("links");
  renderAll();
}

function setMetrics() {
  metricDiscovered.textContent = String(reportData.summary?.links_discovered ?? 0);
  metricOccurrences.textContent = String(reportData.summary?.link_occurrences ?? 0);
  metricDead.textContent = String(reportData.summary?.dead_links ?? 0);
  metricNon200.textContent = String(reportData.summary?.non_200_links ?? 0);
  metricSkipped.textContent = String(reportData.summary?.skipped_links ?? 0);
  metricOk.textContent = String(reportData.summary?.ok_links ?? 0);
  const generated = reportData.generated_at ? new Date(reportData.generated_at).toLocaleString() : "n/a";
  summary.textContent = `Generated ${generated} | entry ${reportData.entry_url || "unknown"} | scope ${reportData.scope?.origin || "unknown"}`;
}

function bindControls() {
  filterSelect.addEventListener("change", renderAll);
  sortSelect.addEventListener("change", renderAll);
  searchInput.addEventListener("input", debounce(renderAll, 120));
  viewLinksBtn.addEventListener("click", () => setView("links"));
  viewSkippedBtn.addEventListener("click", () => setView("skipped"));
}

function setView(view) {
  const isLinks = view === "links";
  linksView.classList.toggle("hidden", !isLinks);
  skippedView.classList.toggle("hidden", isLinks);
  viewLinksBtn.classList.toggle("active", isLinks);
  viewSkippedBtn.classList.toggle("active", !isLinks);
  renderAll();
}

function renderAll() {
  if (!reportData) {
    return;
  }

  if (!linksView.classList.contains("hidden")) {
    renderLinks();
  }
  if (!skippedView.classList.contains("hidden")) {
    renderSkipped();
  }
}

function renderLinks() {
  const term = (searchInput.value || "").trim().toLowerCase();
  const filter = filterSelect.value;
  const sort = sortSelect.value;

  let list = reportData.links.filter((link) => matchesFilter(link, filter, term));
  list = sortLinks(list, sort);

  linksTbody.textContent = "";
  linksEmpty.classList.toggle("hidden", list.length > 0);

  for (const link of list) {
    const row = document.createElement("tr");
    row.dataset.url = link.url;

    const status = buildStatus(link);
    const sourceCount = Array.isArray(link.sources) ? link.sources.length : 0;
    const copyURLVisible = copySupport ? "" : "hidden";
    const firstSource = firstSourceURL(link.sources);

    row.innerHTML = `
      <td>
        <div class="url-main">${linkAnchorHTML(link.url)}</div>
        ${link.fetch_url && link.fetch_url !== link.url ? `<div class="url-sub">Fetches as ${escapeText(link.fetch_url)}</div>` : ""}
      </td>
      <td>
        <span class="status ${status.class}">${status.label}</span>
        ${statusDetailHTML(link)}
      </td>
      <td>
        <strong>${escapeText(String(link.count || 0))}</strong>
        <div class="source-meta">${escapeText(String(sourceCount))} source ${sourceCount === 1 ? "page" : "pages"}</div>
      </td>
      <td>${sourcePreviewHTML(link.sources, 2)}</td>
      <td class="actions">
        <button class="action-btn copy-url-btn" type="button" ${copyURLVisible} data-url="${escapeAttribute(link.url)}">Copy URL</button>
        <button class="action-btn copy-source-btn" type="button" ${copyURLVisible} data-source="${escapeAttribute(firstSource)}" ${firstSource ? "" : "disabled"}>Copy source</button>
      </td>
    `;

    row.addEventListener("click", () => {
      renderDetail(link);
    });

    row.querySelector(".copy-url-btn").addEventListener("click", (event) => {
      event.stopPropagation();
      copyToClipboard(link.url);
    });

    row.querySelector(".copy-source-btn").addEventListener("click", (event) => {
      event.stopPropagation();
      copyToClipboard(firstSource);
    });

    row.querySelector(".url-main a").addEventListener("click", (event) => {
      event.stopPropagation();
    });

    linksTbody.appendChild(row);
  }
}

function renderSkipped() {
  const term = (searchInput.value || "").trim().toLowerCase();
  const sort = sortSelect.value;
  let list = reportData.skipped_links.filter((item) => matchesSkippedSearch(item, term));
  list = sortSkipped(list, sort);

  skippedTbody.textContent = "";
  skippedEmpty.classList.toggle("hidden", list.length > 0);

  for (const item of list) {
    const sourceCount = item.sources ? item.sources.length : 0;
    const firstSource = firstSourceURL(item.sources);
    const copyVisible = copySupport ? "" : "hidden";
    const row = document.createElement("tr");
    row.innerHTML = `
      <td><div class="url-main">${linkAnchorHTML(item.url)}</div></td>
      <td>${escapeText(item.reason || "")}</td>
      <td>
        <strong>${escapeText(String(item.count || 0))}</strong>
        <div class="source-meta">${escapeText(String(sourceCount))} source ${sourceCount === 1 ? "page" : "pages"}</div>
      </td>
      <td>${sourcePreviewHTML(item.sources, 3)}</td>
      <td class="actions">
        <button class="action-btn copy-url-btn" type="button" ${copyVisible} data-url="${escapeAttribute(item.url)}">Copy URL</button>
        <button class="action-btn copy-source-btn" type="button" ${copyVisible} data-source="${escapeAttribute(firstSource)}" ${firstSource ? "" : "disabled"}>Copy source</button>
      </td>
    `;

    row.querySelector(".copy-url-btn").addEventListener("click", (event) => {
      event.stopPropagation();
      copyToClipboard(item.url);
    });

    row.querySelector(".copy-source-btn").addEventListener("click", (event) => {
      event.stopPropagation();
      copyToClipboard(firstSource);
    });

    row.querySelector(".url-main a").addEventListener("click", (event) => {
      event.stopPropagation();
    });

    skippedTbody.appendChild(row);
  }
}

function renderDetail(link) {
  detailEmpty.classList.add("hidden");
  detailContent.classList.remove("hidden");

  const status = buildStatus(link);
  const fetch = fetchesByURL.get(link.fetch_url || "") || {};

  linksDetailURL.innerText = link.url;
  linksDetailFetch.innerText = link.fetch_url || "";
  linksDetailStatusCode.innerText = fetch.status_code || link.status_code || "n/a";
  linksDetailFinal.innerText = link.final_url || "n/a";
  linksDetailContentType.innerText = fetch.content_type || link.content_type || "n/a";
  linksDetailProblem.innerText = link.problem || "none";
  linksDetailError.innerText = fetch.error || "none";
  linksDetailStatus.innerText = status.label;
  linksDetailRedirect.innerText = (fetch.redirect_chain || []).length
    ? fetch.redirect_chain.join(" -> ")
    : "n/a";

  detailSources.textContent = "";
  if (!Array.isArray(link.sources) || link.sources.length === 0) {
    const item = document.createElement("li");
    item.textContent = "No source metadata available.";
    detailSources.appendChild(item);
    return;
  }

  for (const source of link.sources) {
    const item = document.createElement("li");
    item.className = "source-item";

    const page = document.createElement("a");
    page.href = source.page_url;
    page.rel = "noreferrer noopener";
    page.target = "_blank";
    page.textContent = source.page_url || "(unknown)";
    page.title = source.page_url || "";

    const copy = document.createElement("button");
    copy.type = "button";
    copy.className = "copy-btn";
    copy.textContent = "Copy source page";
    if (!copySupport) {
      copy.classList.add("hidden");
    } else {
      copy.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        copyToClipboard(source.page_url || "");
      });
    }

    item.appendChild(page);
    if (source.texts && source.texts.length) {
      const text = document.createElement("span");
      text.className = "source-texts";
      text.textContent = ` - ${source.texts.slice(0, 3).join(" | ")}`;
      item.appendChild(text);
    }
    if (source.count) {
      const usage = document.createElement("small");
      usage.textContent = ` (${source.count} occurrences)`;
      item.appendChild(usage);
    }
    item.appendChild(copy);
    detailSources.appendChild(item);
  }
}

function matchesFilter(link, filter, term) {
  if (term) {
    const inURL = (link.url || "").toLowerCase().includes(term);
    const inFetchURL = (link.fetch_url || "").toLowerCase().includes(term);
    const inFinalURL = (link.final_url || "").toLowerCase().includes(term);
    const inSources = Array.isArray(link.sources) && link.sources.some((source) => sourceMatchesTerm(source, term));
    if (!inURL && !inFetchURL && !inFinalURL && !inSources) {
      return false;
    }
  }

  switch (filter) {
    case "all":
      return true;
    case "dead":
      return !!link.dead;
    case "non200":
      return !!link.non_200;
    case "redirecting":
      return !!link.final_url && link.final_url !== link.fetch_url;
    case "ok":
      return !!link.ok;
    default:
      return true;
  }
}

function matchesSkippedSearch(item, term) {
  if (!term) return true;
  const hasURL = (item.url || "").toLowerCase().includes(term);
  const hasReason = (item.reason || "").toLowerCase().includes(term);
  const hasSource = Array.isArray(item.sources) && item.sources.some((source) => sourceMatchesTerm(source, term));
  return hasURL || hasReason || hasSource;
}

function sortLinks(list, sort) {
  const copy = [...list];
  copy.sort((a, b) => {
    switch (sort) {
      case "count-asc":
        return (a.count || 0) - (b.count || 0);
      case "source-count-asc":
        return (a.sources?.length || 0) - (b.sources?.length || 0);
      case "source-count-desc":
        return (b.sources?.length || 0) - (a.sources?.length || 0);
      case "count-desc":
        return (b.count || 0) - (a.count || 0);
      case "status":
        return statusWeight(a) - statusWeight(b) || (a.url || "").localeCompare(b.url || "");
      case "url":
      default:
        return (a.url || "").localeCompare(b.url || "");
    }
  });
  return copy;
}

function sortSkipped(list, sort) {
  return [...list].sort((a, b) => {
    switch (sort) {
      case "count-asc":
        return (a.count || 0) - (b.count || 0);
      case "count-desc":
        return (b.count || 0) - (a.count || 0);
      case "source-count-asc":
        return (a.sources?.length || 0) - (b.sources?.length || 0);
      case "source-count-desc":
        return (b.sources?.length || 0) - (a.sources?.length || 0);
      case "status":
        return (a.reason || "").localeCompare(b.reason || "") || (a.url || "").localeCompare(b.url || "");
      case "url":
      default:
        return (a.url || "").localeCompare(b.url || "");
    }
  });
}

function statusWeight(link) {
  const status = buildStatus(link).label;
  if (status === "dead") return 0;
  if (status === "non-200") return 1;
  if (status === "redirecting") return 2;
  if (status === "ok") return 3;
  return 4;
}

function buildStatus(link) {
  if (link.dead) {
    return { label: "dead", class: "dead" };
  }
  if (link.non_200) {
    return { label: "non-200", class: "non200" };
  }
  if (link.final_url && link.fetch_url && link.final_url !== link.fetch_url) {
    return { label: "redirecting", class: "redirect" };
  }
  if (link.ok) {
    return { label: "ok", class: "ok" };
  }
  return { label: "unknown", class: "" };
}

function statusDetailHTML(link) {
  const fetch = fetchesByURL.get(link.fetch_url || "") || {};
  const notes = [];
  const code = fetch.status_code || link.status_code;
  const finalURL = link.final_url || fetch.final_url || "";
  const error = fetch.error || link.error || "";

  if (code) {
    notes.push(`HTTP ${code}`);
  }
  if (finalURL && finalURL !== (link.fetch_url || "")) {
    notes.push(`Final: ${finalURL}`);
  }
  if (link.problem) {
    notes.push(link.problem);
  }
  if (error) {
    notes.push(error);
  }

  if (!notes.length) {
    return "";
  }
  return `<div class="status-note">${escapeText(notes.join(" | "))}</div>`;
}

function sourcePreviewHTML(sources, limit) {
  if (!Array.isArray(sources) || sources.length === 0) {
    return '<span class="muted">No source metadata available.</span>';
  }

  const visible = sources.slice(0, limit);
  const rows = visible.map((source) => {
    const count = source.count ? ` (${source.count}x)` : "";
    const snippets = Array.isArray(source.texts) ? source.texts.filter(Boolean).slice(0, 2) : [];
    const snippetHTML = snippets.length
      ? snippets.map((snippet) => `<div class="snippet">${escapeText(snippet)}</div>`).join("")
      : '<div class="snippet">No link text captured.</div>';

    return `
      <div class="source-preview-item">
        <div class="source-page-line">
          <a href="${escapeAttribute(source.page_url || "")}" rel="noreferrer noopener" target="_blank">${escapeText(source.page_url || "(unknown)")}</a>
          <span class="source-meta">${escapeText(count)}</span>
        </div>
        ${snippetHTML}
      </div>
    `;
  });

  const remaining = sources.length - visible.length;
  if (remaining > 0) {
    rows.push(`<div class="more-sources">+${escapeText(String(remaining))} more source ${remaining === 1 ? "page" : "pages"} in detail</div>`);
  }

  return `<div class="source-preview">${rows.join("")}</div>`;
}

function linkAnchorHTML(url) {
  if (!url) {
    return "";
  }
  return `<a href="${escapeAttribute(url)}" rel="noreferrer noopener" target="_blank">${escapeText(url)}</a>`;
}

function firstSourceURL(sources) {
  if (!Array.isArray(sources) || sources.length === 0) {
    return "";
  }
  return sources.find((source) => source.page_url)?.page_url || "";
}

function sourceMatchesTerm(source, term) {
  const inPage = (source.page_url || "").toLowerCase().includes(term);
  const inText = Array.isArray(source.texts) && source.texts.some((text) => (text || "").toLowerCase().includes(term));
  return inPage || inText;
}

function copyToClipboard(value) {
  if (!copySupport || !value) {
    return;
  }
  navigator.clipboard.writeText(value).catch(() => {});
}

function escapeText(value) {
  const span = document.createElement("span");
  span.textContent = value;
  return span.innerHTML;
}

function escapeAttribute(value) {
  return (value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("\"", "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function debounce(fn, wait) {
  let timer = null;
  return () => {
    clearTimeout(timer);
    timer = setTimeout(fn, wait);
  };
}

window.addEventListener("DOMContentLoaded", main);

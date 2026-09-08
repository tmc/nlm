(function () {
  "use strict";
  var root = document.getElementById("root");
  var data = JSON.parse(document.getElementById("chat-data").textContent);
  var heading = document.querySelector(".answer h3, .answer h4");
  if (heading && (!data.title || data.title === "NotebookLM conversation")) {
    document.querySelector("header.doc h1").textContent = heading.textContent;
    document.title = heading.textContent;
  }
  document.querySelectorAll(".table-scroll").forEach(function (wrapper) {
    function sizeTable() {
      var scrollable = wrapper.scrollWidth > wrapper.clientWidth;
      wrapper.classList.toggle("scrollable", scrollable);
      if (scrollable) {
        wrapper.tabIndex = 0;
        wrapper.setAttribute("role", "region");
        wrapper.setAttribute("aria-label", "Table; scroll horizontally for all columns");
      } else {
        wrapper.removeAttribute("tabindex");
        wrapper.removeAttribute("role");
        wrapper.removeAttribute("aria-label");
      }
    }
    sizeTable();
    new ResizeObserver(sizeTable).observe(wrapper);
  });
  function element(tag, cls, text) {
    var node = document.createElement(tag);
    node.className = cls;
    if (text != null) node.textContent = text;
    return node;
  }
  var turns = Array.from(document.querySelectorAll(".turn"));
  var sections = turns.filter(function (turn) { return turn.classList.contains("assistant"); });
  var total = data.messages.reduce(function (n, msg) { return n + (msg.markers || []).length; }, 0);
  document.querySelector("header.doc .sub").textContent = sections.length + " section" + (sections.length === 1 ? "" : "s") + " · " + total + " citations";
  if (heading && document.title === heading.textContent && !heading.querySelector(".citelink, .grounded")) heading.hidden = true;
  var requestNumber = 0;
  turns.forEach(function (turn) {
    if (!turn.classList.contains("user")) return;
    requestNumber++;
    var request = element("details", "report-request");
    request.appendChild(element("summary", "", "Request for section " + requestNumber));
    var body = turn.querySelector(".bubble");
    body.className = "request-text";
    request.appendChild(body);
    turn.replaceChildren(request);
  });
  sections.forEach(function (turn, i) {
    turn.querySelector(".role").textContent = "Section " + (i + 1);
    turn.querySelectorAll(".turn-nav").forEach(function (nav) { nav.remove(); });
    var nav = element("nav", "section-nav");
    nav.setAttribute("aria-label", "Section " + (i + 1) + " navigation");
    function link(label, id) {
      var a = element("a", "", label);
      a.href = "#" + id;
      nav.appendChild(a);
    }
    if (i > 0) link("Previous section", sections[i - 1].id);
    if (i + 1 < sections.length) link("Next section", sections[i + 1].id);
    link("Contents", "message-navigation");
    turn.appendChild(nav);
  });
  var sidebar = element("aside", "conversation-nav");
  sidebar.id = "message-navigation";
  sidebar.setAttribute("aria-label", "Report contents");
  var brand = element("div", "reader-brand");
  brand.appendChild(element("span", "", "REPORT"));
  brand.appendChild(element("span", "reader-origin", "NotebookLM"));
  sidebar.appendChild(brand);
  var index = element("details", "message-index");
  var mobile = window.matchMedia("(max-width: 700px)");
  index.open = !mobile.matches;
  index.appendChild(element("summary", "", "Contents"));
  var list = element("nav", "message-list");
  list.setAttribute("aria-label", "Report sections");
  var links = [];
  if (!sections.length) sections = turns;
  sections.forEach(function (turn, i) {
    var link = element("a", "message-item");
    link.href = "#" + turn.id;
    link.appendChild(element("span", "message-number", String(i + 1).padStart(2, "0")));
    var body = turn.querySelector(".answer, .request-text");
    var title = turn.querySelector(".answer h3, .answer h4, .answer strong");
    var preview = (title ? title.textContent : body.innerText).replace(/\s+/g, " ").trim();
    link.appendChild(element("span", "message-preview", preview.slice(0, 140) || "Section " + (i + 1)));
    link.setAttribute("aria-label", "Section " + (i + 1) + ": " + preview.slice(0, 140));
    link.addEventListener("click", function () { if (mobile.matches) index.open = false; });
    list.appendChild(link);
    links.push(link);
  });
  index.appendChild(list);
  sidebar.appendChild(index);
  root.before(sidebar);
  document.querySelectorAll('.section-nav a[href="#message-navigation"]').forEach(function (link) {
    link.href = "#message-navigation";
    link.textContent = "Contents";
    link.addEventListener("click", function (event) {
      event.preventDefault();
      index.open = true;
      index.querySelector("summary").focus();
    });
  });
  var previous = -1;
  function update() {
    var current = 0;
    var top = mobile.matches ? 90 : 36;
    sections.forEach(function (turn, i) { if (turn.getBoundingClientRect().top <= top) current = i; });
    var target = document.getElementById(location.hash.slice(1));
    var named = target && target.closest(".turn");
    if (named && named.classList.contains("user")) named = named.nextElementSibling;
    if (named && sections.includes(named)) {
      var rect = named.getBoundingClientRect();
      if (rect.top >= 0 && rect.top < innerHeight) current = sections.indexOf(named);
    }
    if (current === previous || !links[current]) return;
    if (links[previous]) links[previous].removeAttribute("aria-current");
    links[current].setAttribute("aria-current", "location");
    previous = current;
    var item = links[current].getBoundingClientRect();
    var box = list.getBoundingClientRect();
    if (item.top < box.top || item.bottom > box.bottom) list.scrollTop += item.top - box.top - box.height / 3;
  }
  index.addEventListener("toggle", function () {
    if (index.open) { previous = -1; update(); }
  });
  var pending = false;
  window.addEventListener("scroll", function () {
    if (pending) return;
    pending = true;
    requestAnimationFrame(function () { pending = false; update(); });
  }, {passive: true});
  window.addEventListener("hashchange", update);
  mobile.addEventListener("change", function () { index.open = !mobile.matches; update(); });
  update();
  var rails = Array.from(document.querySelectorAll("details.rail"));
  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape") return;
    rails.forEach(function (rail) {
      if (!rail.open) return;
      rail.open = false;
      rail.querySelector("summary").focus({preventScroll: true});
    });
    if (mobile.matches && index.open) {
      index.open = false;
      index.querySelector("summary").focus();
    }
  });
})();

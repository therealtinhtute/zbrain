(function () {
  "use strict";

  var reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var navToggle = document.getElementById("nav-toggle");
  var navLinks = document.getElementById("nav-links");

  if (navToggle && navLinks) {
    function setExpanded(expanded) {
      navLinks.classList.toggle("open", expanded);
      navToggle.setAttribute("aria-expanded", String(expanded));
    }

    navToggle.addEventListener("click", function () {
      setExpanded(navToggle.getAttribute("aria-expanded") !== "true");
    });

    navLinks.addEventListener("click", function (event) {
      if (event.target.closest("a")) setExpanded(false);
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape") setExpanded(false);
    });
  }

  var current = document.location.pathname.split("/").pop() || "index.html";
  document.querySelectorAll(".nav-mast__links a").forEach(function (link) {
    if (link.getAttribute("href").split("/").pop() === current) {
      link.classList.add("active");
    }
  });

  if (!reduced) {
    document.querySelectorAll('a[href^="#"]').forEach(function (link) {
      link.addEventListener("click", function (event) {
        var target = document.querySelector(link.getAttribute("href"));
        if (target) {
          event.preventDefault();
          target.scrollIntoView({ behavior: "smooth", block: "start" });
        }
      });
    });
  }
})();
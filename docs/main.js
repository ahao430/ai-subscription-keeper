// 滚动渐显动画（IntersectionObserver，纯 JS 无依赖）
// 渐进增强：无 JS 或观察器异常时内容依然完整可见
(function () {
  document.documentElement.classList.add('js');
  var els = Array.prototype.slice.call(document.querySelectorAll('.reveal'));

  function revealAll() {
    els.forEach(function (el) { el.classList.add('visible'); });
  }

  if (!('IntersectionObserver' in window)) {
    revealAll();
    return;
  }

  var io = new IntersectionObserver(
    function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add('visible');
          io.unobserve(entry.target);
        }
      });
    },
    { threshold: 0.12 }
  );
  els.forEach(function (el) { io.observe(el); });

  // 兜底：3 秒后若一个都未显示（如观察器被环境抑制），全部直接显示
  setTimeout(function () {
    if (!document.querySelector('.reveal.visible')) revealAll();
  }, 3000);
})();

import { useEffect } from "react";

// Fill colour is primary teal; empty colour comes from CSS var so it matches light/dark theme
const GRADIENT_FILL = "#003d48";

export function ScrollProgress() {
  useEffect(() => {
    const handleScroll = () => {
      const pct = (window.scrollY / (document.body.scrollHeight - window.innerHeight)) * 100;
      const scrollBar = document.querySelector('.scroll-bar') as HTMLElement;
      if (scrollBar) {
        scrollBar.style.setProperty('--pct', pct + '%');
      }
    };

    window.addEventListener('scroll', handleScroll);
    handleScroll(); // Initial call

    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <div
      className="fixed top-0 left-0 right-0 z-50 h-px scroll-bar"
      style={{ background: `linear-gradient(to right, ${GRADIENT_FILL} var(--pct, 0%), var(--scroll-progress-empty) var(--pct, 0%))` }}
    />
  );
}

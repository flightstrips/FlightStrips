import { useEffect } from "react";

// CSS variable–driven gradient (hex must be literal strings here — Tailwind JIT cannot handle CSS variables)
const GRADIENT_FILL  = "#003d48"; // teal fill colour
const GRADIENT_EMPTY = "#F3EEE8"; // off-white empty colour

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
      style={{ background: `linear-gradient(to right, ${GRADIENT_FILL} var(--pct, 0%), ${GRADIENT_EMPTY} var(--pct, 0%))` }}
    />
  );
}

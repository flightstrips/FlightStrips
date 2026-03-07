type MetarHelperProps = {
  metar: string | null;
  style?: "full" | "winds" | "temp" | "conditions" | "qnh";
  unit?: "hPa" | "inHg";
};

export default function MetarHelper({ metar, style = "full", unit = "hPa" }: MetarHelperProps) {
  const getContent = () => {
    if (!metar) return "N/A";

    switch (style) {
      case "winds": {
        // Handle calm (00000KT), variable (VRB05KT), gusting (27015G25KT)
        const calmMatch = metar.match(/\b00000KT\b/);
        if (calmMatch) return "Calm";

        const vrbMatch = metar.match(/\bVRB(\d{2})(?:G\d{2})?KT\b/);
        if (vrbMatch) return `VRB ${vrbMatch[1]}KT`;

        const windMatch = metar.match(/\b(\d{3})(\d{2})(?:G(\d{2}))?KT\b/);
        if (windMatch) {
          const degrees = windMatch[1];
          const speed = windMatch[2];
          const gust = windMatch[3];
          return gust ? `${degrees}° ${speed}G${gust}KT` : `${degrees}° ${speed}KT`;
        }
        return "No wind info";
      }
      case "temp": {
        const tempMatch = metar.match(/\b(M?\d{2})\/(M?\d{2})\b/);
        return tempMatch ? `${tempMatch[0]}` : "N/A";
      }
      case "conditions": {
        const condMatch = metar.match(/\b(VCSH|RA|SN|FG|BR|HZ|TS)\b/);
        return condMatch ? condMatch[0] : "N/A";
      }
      case "qnh": {
        // Match QNH1013, Q1013, or A2992
        const qnhMatch = metar.match(/\b(?:QNH(\d{4})|Q(\d{4})|A(\d{4}))\b/);
        if (qnhMatch) {
          if (qnhMatch[1] || qnhMatch[2]) {
            const hpa = parseInt(qnhMatch[1] ?? qnhMatch[2], 10);
            if (unit === "inHg") {
              return (hpa * 0.02953).toFixed(2);
            }
            return String(hpa);
          } else if (qnhMatch[3]) {
            // A-prefix: value in hundredths of inHg
            const inHg = parseInt(qnhMatch[3], 10) / 100;
            if (unit === "hPa") {
              return Math.round(inHg / 0.02953).toString();
            }
            return inHg.toFixed(2);
          }
        }
        return "N/A";
      }
      case "full":
      default:
        return metar;
    }
  };

  const content = getContent();

  return <div>{content}</div>;
}
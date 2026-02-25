type MetarHelperProps = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  metar: any;
  style?: "full" | "winds" | "temp" | "conditions" | "qnh";
  unit?: "hPa" | "inHg";
};

export default function MetarHelper({ metar, style = "full", unit = "hPa" }: MetarHelperProps) {
  console.log(metar.props);
  const getContent = () => {
    switch (style) {
      case "winds": {
        const windMatch = metar.match(/\b(\d{3})(\d{2})KT\b/);
        if (windMatch) {
          const degrees = windMatch[1];
          const speed = windMatch[2];
          return `${degrees}° ${speed}KT`;
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
        const qnhMatch = metar.match(/\b(Q\d{4}|A\d{4})\b/);
        if (qnhMatch) {
          const raw = qnhMatch[0];
          if (raw.startsWith("Q")) {
            const hpa = parseInt(raw.substring(1), 10);
            if (unit === "inHg") {
              return (hpa * 0.02953).toFixed(2);
            }
            return String(hpa);
          } else {
            // A-prefix: value in hundredths of inHg
            const inHg = parseInt(raw.substring(1), 10) / 100;
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
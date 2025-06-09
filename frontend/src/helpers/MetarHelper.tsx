type MetarHelperProps = {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    metar: any;
    style?: 'full' | 'winds' | 'temp' | 'conditions' | 'qnh';
};

export default function MetarHelper({ metar, style = 'full' }: MetarHelperProps) {
    console.log(metar.props)
    // Placeholder parsing logic
    const getContent = () => {
        switch (style) {
            case 'winds':
                // Example: extract wind info (very basic)
                {
                    const windMatch = metar.match(/\b(\d{3})(\d{2})KT\b/);
                    if (windMatch) {
                        const degrees = windMatch[1];
                        const speed = windMatch[2];
                        return `${degrees}° ${speed}KT`;
                    }
                    return 'No wind info';
                }
            case 'temp':
                // Example: extract temperature info (very basic)
                { const tempMatch = metar.match(/\b(M?\d{2})\/(M?\d{2})\b/);
                return tempMatch ? `${tempMatch[0]}` : 'N/A'; }
            case 'conditions':
                // Example: extract weather conditions (very basic)
                { const condMatch = metar.match(/\b(VCSH|RA|SN|FG|BR|HZ|TS)\b/);
                return condMatch ? condMatch[0] : 'N/A'; }
            case 'full':
            default:
                return metar;
            case 'qnh':
                // Example: extract QNH (altimeter setting, e.g., Q1013 or A2992)
                {
                    const qnhMatch = metar.match(/\b(Q\d{4}|A\d{4})\b/);
                    if (qnhMatch) {
                        // Remove the leading 'Q' or 'A'
                        return qnhMatch[0].substring(1);
                    }
                    return 'N/A';
                }
        }
    };

    const content = getContent();

    return (
        <div>
            {content}
        </div>
    );
}
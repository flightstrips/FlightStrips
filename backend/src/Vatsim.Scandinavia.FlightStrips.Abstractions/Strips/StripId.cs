namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public readonly record struct StripId
{
    public StripId(string Session, string Airport, string Callsign)
    {
        this.Session = Session.ToUpperInvariant();
        this.Airport = Airport.ToUpperInvariant();
        this.Callsign = Callsign.ToUpperInvariant();
    }

    public string Session { get; }
    public string Airport { get; }
    public string Callsign { get; }

    public override string ToString()
    {
        return $"{Session}:{Airport}:{Callsign}";
    }
};
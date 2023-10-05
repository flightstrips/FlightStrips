namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public readonly record struct StripId(string Session, string Airport, string Callsign)
{
    public override string ToString()
    {
        return $"{Session}:{Airport}:{Callsign}";
    }
};
namespace Vatsim.Scandinavia.FlightStrips.Abstractions;

public readonly record struct SessionId(string Airport, string Session)
{
    public string Airport { get; } = Airport.ToUpperInvariant();
    public string Session { get; } = Session.ToUpperInvariant();
}

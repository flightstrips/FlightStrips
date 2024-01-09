namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public readonly record struct OnlinePositionId(string Airport, string Session, string Position)
{
    public string Airport { get; } = Airport.ToUpperInvariant();
    public string Session { get; } = Session.ToUpperInvariant();
}

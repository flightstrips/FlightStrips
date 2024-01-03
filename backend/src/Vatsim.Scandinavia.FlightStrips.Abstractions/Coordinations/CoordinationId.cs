namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public readonly record struct CoordinationId(string Airport, string Session, int Id)
{
    public string Airport { get; } = Airport.ToUpperInvariant();
    public string Session { get; } = Session.ToUpperInvariant();
}

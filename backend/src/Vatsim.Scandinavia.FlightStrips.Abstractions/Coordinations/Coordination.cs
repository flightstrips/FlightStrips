namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Coordinations;

public class Coordination
{
    public int Id { get; set; }

    public CoordinationState State { get; set; } = CoordinationState.Transfer;

    public required string Callsign { get; set; }

    public required string FromFrequency { get; set; }
    public required string ToFrequency { get; set; }
}

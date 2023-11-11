namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class CoordinationUpdateModel
{
    public int CoordinationId { get; set; }

    public required string Callsign { get; set; }

    public required string To { get; set; }

    public required string From { get; set; }

    public required CoordinationState State { get; set; }
}

public enum CoordinationState
{
    Created,
    Accepted,
    Rejected,
    Cancelled
}

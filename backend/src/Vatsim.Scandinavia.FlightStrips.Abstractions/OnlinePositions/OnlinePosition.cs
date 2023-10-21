namespace Vatsim.Scandinavia.FlightStrips.Abstractions.OnlinePositions;

public class OnlinePosition
{
    public required string PositionId { get; set; }
    
    public required string PrimaryFrequency { get; set; }
}
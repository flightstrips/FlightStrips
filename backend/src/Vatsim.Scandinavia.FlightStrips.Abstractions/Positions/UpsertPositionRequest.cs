namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Positions;

public class UpsertPositionRequest
{
    public required string Name { get; set; }
    
    public required string Frequency { get; set; }
}
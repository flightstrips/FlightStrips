namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class StripPositionUpdate
{
    public required string Callsign { get; set; }

    public required double Latitude { get; set; }
    public required double Longitude { get; set; }
    public required double Altitude { get; set; }
}

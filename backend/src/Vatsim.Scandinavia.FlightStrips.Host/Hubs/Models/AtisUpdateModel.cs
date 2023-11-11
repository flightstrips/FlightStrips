namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class AtisUpdateModel
{
    public required char Letter { get; set; }

    public required string Callsign { get; set; }

    public required string Metar { get; set; }
}

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class SectorUpdateModel
{
    public required string Frequency { get; set; }

    public required string[] Sectors { get; set; }

}

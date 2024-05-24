using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class SubscribeAirportModel
{
    [Required]
    public required string Airport { get; set; }

    [Required]
    public required string Session { get; set; }

    public bool IncludePositionUpdates { get; set; }
}

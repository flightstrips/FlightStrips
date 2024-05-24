using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class StripDisconnectedModel
{
    [Required]
    public required string Callsign { get; set; }
}

using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class UnsubscribeModel
{
    [Required]
    public required string Airport { get; set; }

    [Required]
    public required string Session { get; set; }

    [Required]
    public required string Callsign { get; set; }

    public bool UnsubscribeFromAirport { get; set; }
}

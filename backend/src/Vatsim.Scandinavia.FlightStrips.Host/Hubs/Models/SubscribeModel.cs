using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class SubscribeModel
{
    [Required]
    public required string Airport { get; set; }

    [Required]
    public required string Session { get; set; }

    [Required]
    public required string Frequency { get; set; }
}

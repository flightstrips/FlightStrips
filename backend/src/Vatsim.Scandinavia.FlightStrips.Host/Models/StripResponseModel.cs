using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class StripResponseModel
{
    [Required]
    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public bool Cleared { get; set; }

    public string? Controller { get; set; }

    [Required]
    public required string Bay { get; set; } = string.Empty;
}

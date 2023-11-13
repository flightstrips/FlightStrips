using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripAssumeRequestModel
{
    [Required]
    public required string Frequency { get; set; }

    [Required]
    public bool Force { get; set; }
}

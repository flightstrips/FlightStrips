using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripTransferRequestModel
{
    [Required]
    [Frequency]
    public required string CurrentFrequency { get; set; }

    [Required]
    [Frequency]
    public required string ToFrequency { get; set; }
}

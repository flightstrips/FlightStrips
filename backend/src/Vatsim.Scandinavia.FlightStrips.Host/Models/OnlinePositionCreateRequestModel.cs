using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class OnlinePositionCreateRequestModel
{
    [Required]
    [Frequency]
    public string Frequency { get; set; } = string.Empty;
}

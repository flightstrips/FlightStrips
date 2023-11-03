using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class AcceptCoordinationRequestModel
{
    [Required, Frequency]
    public required string Frequency { get; set; }
}

using System.ComponentModel.DataAnnotations;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class RejectCoordinationRequestModel
{
    [Required, Frequency]
    public required string Frequency { get; set; }
}

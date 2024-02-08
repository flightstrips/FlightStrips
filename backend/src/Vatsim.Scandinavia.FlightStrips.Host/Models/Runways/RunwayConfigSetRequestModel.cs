using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models.Runways;

public class RunwayConfigSetRequestModel
{
    [Required] public string Departure { get; set; } = string.Empty;

    [Required] public string Arrival { get; set; } = string.Empty;

    [Required] public string Position { get; set; } = string.Empty;
}

using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripMoveRequestModel
{
    [Required]
    public string Bay { get; set; } = string.Empty;

    public int? Sequence { get; set; }
}

using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class UpsertPositionRequestModel
{
    [Required] [MaxLength(50)] public string Name { get; set; } = string.Empty;

}
